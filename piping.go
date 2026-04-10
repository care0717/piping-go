package main

import (
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"sync"
)

type receiver struct {
	w       http.ResponseWriter
	flusher http.Flusher
	done    chan struct{} // closed when streaming to this receiver is complete
}

type pipe struct {
	nReceivers int

	// sender fields
	senderBody    io.Reader
	senderHeaders http.Header
	senderReady   chan struct{} // closed when sender arrives

	// receiver fields
	mu            sync.Mutex
	receivers     []*receiver
	receiverReady chan struct{} // closed when all receivers arrive
	receiverOnce  sync.Once

	aborted     chan struct{} // closed when transfer cannot proceed
	abortStatus int
	abortMsg    string
}

// PipingServer coordinates data transfer between senders and receivers via shared paths.
type PipingServer struct {
	mu    sync.Mutex
	pipes map[string]*pipe
}

func NewPipingServer() *PipingServer {
	return &PipingServer{
		pipes: make(map[string]*pipe),
	}
}

func (ps *PipingServer) getOrCreatePipe(path string, nReceivers int) (*pipe, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if p, ok := ps.pipes[path]; ok {
		if p.nReceivers != nReceivers {
			return nil, fmt.Errorf("number of receivers mismatch: expected %d, got %d", p.nReceivers, nReceivers)
		}
		return p, nil
	}

	p := &pipe{
		nReceivers:    nReceivers,
		senderReady:   make(chan struct{}),
		receiverReady: make(chan struct{}),
		aborted:       make(chan struct{}),
	}
	ps.pipes[path] = p
	return p, nil
}

func (ps *PipingServer) removePipe(path string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.pipes, path)
}

func getNReceivers(r *http.Request) int {
	nStr := r.URL.Query().Get("n")
	if nStr == "" {
		return 1
	}
	n, err := strconv.Atoi(nStr)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func (p *pipe) markReceiversReady() {
	p.receiverOnce.Do(func() {
		close(p.receiverReady)
	})
}

func (p *pipe) abort(status int, msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	select {
	case <-p.aborted:
		return
	default:
	}

	p.abortStatus = status
	p.abortMsg = msg
	close(p.aborted)

	for _, recv := range p.receivers {
		select {
		case <-recv.done:
		default:
			close(recv.done)
		}
	}
}

// HandleSender handles POST/PUT requests from data senders.
func (ps *PipingServer) HandleSender(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	nReceivers := getNReceivers(r)

	p, err := ps.getOrCreatePipe(path, nReceivers)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check if sender already exists
	p.mu.Lock()
	if p.senderBody != nil {
		p.mu.Unlock()
		http.Error(w, fmt.Sprintf("[ERROR] Another sender is already connected to %s\n", path), http.StatusBadRequest)
		return
	}

	// Enable full-duplex mode so we can read the request body
	// while writing status messages to the response (Go 1.21+).
	// Only enable after we know this sender will be accepted.
	rc := http.NewResponseController(w)
	if err := rc.EnableFullDuplex(); err != nil {
		log.Printf("[WARN] EnableFullDuplex failed: %v\n", err)
	}

	// Extract body and headers.
	body, headers := extractSenderBody(r)
	p.senderBody = body
	p.senderHeaders = headers
	p.mu.Unlock()

	// Signal sender is ready
	close(p.senderReady)

	log.Printf("[INFO] Sender connected to %s\n", path)
	fmt.Fprintf(w, "[INFO] Sender connected to %s\n", path)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Wait for all receivers
	select {
	case <-p.receiverReady:
	case <-r.Context().Done():
		log.Printf("[INFO] Sender disconnected from %s before receivers connected\n", path)
		p.abort(http.StatusGone, "Sender disconnected before all receivers connected")
		ps.removePipe(path)
		return
	}

	log.Printf("[INFO] Start piping to %s (receivers: %d)\n", path, nReceivers)
	fmt.Fprintf(w, "[INFO] Start piping to %s (receivers: %d)\n", path, nReceivers)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Forward headers and build a multi-writer from all receivers.
	// All ResponseWriter access happens in the sender goroutine to avoid races.
	writers := make([]io.Writer, len(p.receivers))
	for i, recv := range p.receivers {
		forwardHeaders(recv.w, p.senderHeaders)
		if recv.flusher != nil {
			writers[i] = &flushWriter{w: recv.w, f: recv.flusher}
		} else {
			writers[i] = recv.w
		}
	}
	mw := io.MultiWriter(writers...)

	// Stream sender body to all receivers
	_, copyErr := io.Copy(mw, p.senderBody)
	if copyErr != nil {
		log.Printf("[WARN] Error during piping on %s: %v\n", path, copyErr)
	}

	// Signal all receivers that streaming is complete
	for _, recv := range p.receivers {
		close(recv.done)
	}

	ps.removePipe(path)
	log.Printf("[INFO] Completed transfer on %s\n", path)
	fmt.Fprintf(w, "[INFO] Completed transfer on %s\n", path)
}

// HandleReceiver handles GET requests from data receivers.
func (ps *PipingServer) HandleReceiver(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	nReceivers := getNReceivers(r)

	// Reject service worker requests
	if r.Header.Get("Service-Worker") == "script" {
		http.Error(w, "Service Worker request not allowed", http.StatusBadRequest)
		return
	}

	p, err := ps.getOrCreatePipe(path, nReceivers)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	flusher, _ := w.(http.Flusher)
	recv := &receiver{
		w:       w,
		flusher: flusher,
		done:    make(chan struct{}),
	}

	// Register receiver
	p.mu.Lock()
	if len(p.receivers) >= p.nReceivers {
		p.mu.Unlock()
		http.Error(w, "Too many receivers connected", http.StatusBadRequest)
		return
	}
	p.receivers = append(p.receivers, recv)
	count := len(p.receivers)
	allReady := count >= p.nReceivers
	p.mu.Unlock()

	if allReady {
		p.markReceiversReady()
	}

	log.Printf("[INFO] Receiver %d/%d connected to %s\n", count, p.nReceivers, path)

	// Wait for sender
	select {
	case <-p.senderReady:
	case <-r.Context().Done():
		log.Printf("[INFO] Receiver disconnected from %s before sender connected\n", path)
		p.mu.Lock()
		for i, rv := range p.receivers {
			if rv == recv {
				p.receivers = append(p.receivers[:i], p.receivers[i+1:]...)
				break
			}
		}
		isEmpty := p.senderBody == nil && len(p.receivers) == 0
		p.mu.Unlock()
		if isEmpty {
			ps.removePipe(path)
		}
		return
	}

	// The sender goroutine writes headers and data to us via io.MultiWriter.
	// Wait for it to finish.
	select {
	case <-recv.done:
		select {
		case <-p.aborted:
			p.mu.Lock()
			status := p.abortStatus
			msg := p.abortMsg
			p.mu.Unlock()
			if status == 0 {
				status = http.StatusGone
			}
			if msg == "" {
				msg = "Transfer aborted"
			}
			http.Error(w, msg, status)
		default:
		}
	case <-r.Context().Done():
		log.Printf("[INFO] Receiver disconnected from %s during transfer\n", path)
	}
}

// extractSenderBody extracts the body and relevant headers from the sender request.
// For multipart/form-data, it extracts the first file part.
func extractSenderBody(r *http.Request) (io.Reader, http.Header) {
	headers := make(http.Header)

	contentType := r.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)

	if err == nil && mediaType == "multipart/form-data" {
		boundary := params["boundary"]
		if boundary != "" {
			reader := multipart.NewReader(r.Body, boundary)
			part, err := reader.NextPart()
			if err == nil {
				if ct := part.Header.Get("Content-Type"); ct != "" {
					headers.Set("Content-Type", ct)
				}
				if cd := part.Header.Get("Content-Disposition"); cd != "" {
					headers.Set("Content-Disposition", cd)
				}
				return part, headers
			}
		}
	}

	// Non-multipart: pass through directly
	if contentType != "" {
		headers.Set("Content-Type", contentType)
	}
	if cl := r.Header.Get("Content-Length"); cl != "" {
		headers.Set("Content-Length", cl)
	}
	if cd := r.Header.Get("Content-Disposition"); cd != "" {
		headers.Set("Content-Disposition", cd)
	}
	if xp := r.Header.Get("X-Piping"); xp != "" {
		headers.Set("X-Piping", xp)
	}

	return r.Body, headers
}

// forwardHeaders copies piping-relevant headers from sender to receiver response.
func forwardHeaders(w http.ResponseWriter, headers http.Header) {
	ct := headers.Get("Content-Type")
	if ct != "" {
		mediaType, params, err := mime.ParseMediaType(ct)
		if err == nil && mediaType == "text/html" {
			ct = mime.FormatMediaType("text/plain", params)
		}
		w.Header().Set("Content-Type", ct)
	}

	if cl := headers.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	if cd := headers.Get("Content-Disposition"); cd != "" {
		w.Header().Set("Content-Disposition", cd)
	}
	if xp := headers.Get("X-Piping"); xp != "" {
		w.Header().Set("X-Piping", xp)
	}
}

// flushWriter wraps a ResponseWriter and flushes after each write for real-time streaming.
type flushWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if n > 0 {
		fw.f.Flush()
	}
	return n, err
}
