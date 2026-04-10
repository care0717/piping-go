package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timeout waiting for condition")
}

func newTestServer() (*PipingServer, *httptest.Server) {
	ps := NewPipingServer()
	rt := &router{piping: ps}
	ts := httptest.NewServer(rt)
	return ps, ts
}

func TestSendAndReceiveText(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	body := "hello piping-go"
	var received string
	var wg sync.WaitGroup
	wg.Add(2)

	// Sender
	go func() {
		defer wg.Done()
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/test1", strings.NewReader(body))
		req.Header.Set("Content-Type", "text/plain")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("sender error: %v", err)
			return
		}
		defer resp.Body.Close()
		io.ReadAll(resp.Body)
	}()

	// Receiver
	go func() {
		defer wg.Done()
		// Small delay so sender registers first
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get(ts.URL + "/test1")
		if err != nil {
			t.Errorf("receiver error: %v", err)
			return
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		received = string(data)
	}()

	wg.Wait()

	if received != body {
		t.Errorf("expected %q, got %q", body, received)
	}
}

func TestReceiverFirstThenSender(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	body := "receiver-first test"
	var received string
	var wg sync.WaitGroup
	wg.Add(2)

	// Receiver first
	go func() {
		defer wg.Done()
		resp, err := http.Get(ts.URL + "/test2")
		if err != nil {
			t.Errorf("receiver error: %v", err)
			return
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		received = string(data)
	}()

	// Sender after a delay
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/test2", strings.NewReader(body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("sender error: %v", err)
			return
		}
		defer resp.Body.Close()
		io.ReadAll(resp.Body)
	}()

	wg.Wait()

	if received != body {
		t.Errorf("expected %q, got %q", body, received)
	}
}

func TestBinaryData(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	// Binary data with null bytes
	data := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD, 0x00, 0x80}
	var received []byte
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/binary", bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("sender error: %v", err)
			return
		}
		defer resp.Body.Close()
		io.ReadAll(resp.Body)
	}()

	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get(ts.URL + "/binary")
		if err != nil {
			t.Errorf("receiver error: %v", err)
			return
		}
		defer resp.Body.Close()
		received, _ = io.ReadAll(resp.Body)
	}()

	wg.Wait()

	if !bytes.Equal(received, data) {
		t.Errorf("binary data mismatch: got %v, want %v", received, data)
	}
}

func TestMultipleReceivers(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	body := "multi-receiver data"
	results := make([]string, 3)
	var wg sync.WaitGroup
	wg.Add(4) // 1 sender + 3 receivers

	// 3 receivers
	for i := 0; i < 3; i++ {
		go func(idx int) {
			defer wg.Done()
			resp, err := http.Get(ts.URL + "/multi?n=3")
			if err != nil {
				t.Errorf("receiver %d error: %v", idx, err)
				return
			}
			defer resp.Body.Close()
			data, _ := io.ReadAll(resp.Body)
			results[idx] = string(data)
		}(i)
	}

	// Sender after receivers connect
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/multi?n=3", strings.NewReader(body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("sender error: %v", err)
			return
		}
		defer resp.Body.Close()
		io.ReadAll(resp.Body)
	}()

	wg.Wait()

	for i, r := range results {
		if r != body {
			t.Errorf("receiver %d: expected %q, got %q", i, body, r)
		}
	}
}

func TestHeaderForwarding(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	var recvHeaders http.Header
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/headers", strings.NewReader("data"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Disposition", `attachment; filename="test.json"`)
		req.Header.Set("X-Piping", "custom-value")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("sender error: %v", err)
			return
		}
		defer resp.Body.Close()
		io.ReadAll(resp.Body)
	}()

	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get(ts.URL + "/headers")
		if err != nil {
			t.Errorf("receiver error: %v", err)
			return
		}
		defer resp.Body.Close()
		recvHeaders = resp.Header
		io.ReadAll(resp.Body)
	}()

	wg.Wait()

	if ct := recvHeaders.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: expected %q, got %q", "application/json", ct)
	}
	if cd := recvHeaders.Get("Content-Disposition"); cd != `attachment; filename="test.json"` {
		t.Errorf("Content-Disposition: expected attachment header, got %q", cd)
	}
	if xp := recvHeaders.Get("X-Piping"); xp != "custom-value" {
		t.Errorf("X-Piping: expected %q, got %q", "custom-value", xp)
	}
}

func TestTextHtmlRewrittenToTextPlain(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	var recvContentType string
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/html", strings.NewReader("<h1>hi</h1>"))
		req.Header.Set("Content-Type", "text/html; charset=utf-8")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("sender error: %v", err)
			return
		}
		defer resp.Body.Close()
		io.ReadAll(resp.Body)
	}()

	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get(ts.URL + "/html")
		if err != nil {
			t.Errorf("receiver error: %v", err)
			return
		}
		defer resp.Body.Close()
		recvContentType = resp.Header.Get("Content-Type")
		io.ReadAll(resp.Body)
	}()

	wg.Wait()

	if !strings.HasPrefix(recvContentType, "text/plain") {
		t.Errorf("expected text/plain, got %q", recvContentType)
	}
}

func TestDuplicateSenderRejected(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	var wg sync.WaitGroup
	wg.Add(3)

	var sender2Status int

	// First sender
	go func() {
		defer wg.Done()
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/dup", strings.NewReader("first"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		io.ReadAll(resp.Body)
	}()

	// Second sender (should be rejected)
	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/dup", strings.NewReader("second"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("sender2 error: %v", err)
			return
		}
		defer resp.Body.Close()
		sender2Status = resp.StatusCode
		io.ReadAll(resp.Body)
	}()

	// Receiver (to unblock first sender)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		resp, err := http.Get(ts.URL + "/dup")
		if err != nil {
			return
		}
		defer resp.Body.Close()
		io.ReadAll(resp.Body)
	}()

	wg.Wait()

	if sender2Status != http.StatusBadRequest {
		t.Errorf("expected 400 for duplicate sender, got %d", sender2Status)
	}
}

func TestNReceiverMismatch(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	// First request creates pipe with n=2 (blocks waiting for sender).
	// Use a context so we can cancel it and unblock the server handler.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/mismatch?n=2", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		io.ReadAll(resp.Body)
	}()

	time.Sleep(50 * time.Millisecond)

	// Second request with n=1 should get 400 mismatch
	resp, err := http.Get(ts.URL + "/mismatch?n=1")
	if err != nil {
		t.Fatalf("second request error: %v", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for n mismatch, got %d", resp.StatusCode)
	}

	cancel()
}

func TestCORSHeaders(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/version")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	defer resp.Body.Close()

	if v := resp.Header.Get("Access-Control-Allow-Origin"); v != "*" {
		t.Errorf("CORS origin: expected *, got %q", v)
	}
	if v := resp.Header.Get("X-Robots-Tag"); v != "none" {
		t.Errorf("X-Robots-Tag: expected none, got %q", v)
	}
}

func TestReservedPaths(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{"/version", 200, version},
		{"/favicon.ico", 204, ""},
		{"/robots.txt", 404, ""},
	}

	for _, tt := range tests {
		resp, err := http.Get(ts.URL + tt.path)
		if err != nil {
			t.Errorf("GET %s error: %v", tt.path, err)
			continue
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if resp.StatusCode != tt.wantStatus {
			t.Errorf("GET %s: status = %d, want %d", tt.path, resp.StatusCode, tt.wantStatus)
		}
		if tt.wantBody != "" && strings.TrimSpace(string(body)) != tt.wantBody {
			t.Errorf("GET %s: body = %q, want %q", tt.path, string(body), tt.wantBody)
		}
	}
}

func TestOptionsPreflightReturns200(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/anypath", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("OPTIONS: expected 200, got %d", resp.StatusCode)
	}
	if v := resp.Header.Get("Access-Control-Allow-Methods"); v == "" {
		t.Error("OPTIONS: missing Access-Control-Allow-Methods")
	}
}

func TestLargeData(t *testing.T) {
	_, ts := newTestServer()
	defer ts.Close()

	// 1MB of data
	size := 1024 * 1024
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	var received []byte
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/large", bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("sender error: %v", err)
			return
		}
		defer resp.Body.Close()
		io.ReadAll(resp.Body)
	}()

	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get(ts.URL + "/large")
		if err != nil {
			t.Errorf("receiver error: %v", err)
			return
		}
		defer resp.Body.Close()
		received, _ = io.ReadAll(resp.Body)
	}()

	wg.Wait()

	if !bytes.Equal(received, data) {
		t.Errorf("large data mismatch: got %d bytes, want %d bytes", len(received), len(data))
	}
}

func TestReceiverOverflowRejectedWithoutPanic(t *testing.T) {
	ps := NewPipingServer()

	p, err := ps.getOrCreatePipe("/overflow", 1)
	if err != nil {
		t.Fatalf("getOrCreatePipe error: %v", err)
	}

	p.receivers = append(p.receivers, &receiver{done: make(chan struct{})})
	p.markReceiversReady()

	req := httptest.NewRequest(http.MethodGet, "/overflow?n=1", nil)
	rec := httptest.NewRecorder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("HandleReceiver panicked: %v", recovered)
		}
	}()

	ps.HandleReceiver(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Too many receivers connected") {
		t.Fatalf("unexpected body: %q", string(body))
	}
}

func TestSenderDisconnectUnblocksWaitingReceiver(t *testing.T) {
	ps := NewPipingServer()

	senderCtx, cancelSender := context.WithCancel(context.Background())
	defer cancelSender()

	senderReq := httptest.NewRequest(http.MethodPost, "/abort?n=2", strings.NewReader("payload")).WithContext(senderCtx)
	senderRec := httptest.NewRecorder()

	senderDone := make(chan struct{})
	go func() {
		defer close(senderDone)
		ps.HandleSender(senderRec, senderReq)
	}()

	waitFor(t, time.Second, func() bool {
		ps.mu.Lock()
		p := ps.pipes["/abort"]
		ps.mu.Unlock()
		if p == nil {
			return false
		}
		select {
		case <-p.senderReady:
			return true
		default:
			return false
		}
	})

	receiverReq := httptest.NewRequest(http.MethodGet, "/abort?n=2", nil)
	receiverRec := httptest.NewRecorder()

	receiverDone := make(chan struct{})
	go func() {
		defer close(receiverDone)
		ps.HandleReceiver(receiverRec, receiverReq)
	}()

	waitFor(t, time.Second, func() bool {
		ps.mu.Lock()
		p := ps.pipes["/abort"]
		ps.mu.Unlock()
		if p == nil {
			return false
		}
		p.mu.Lock()
		count := len(p.receivers)
		p.mu.Unlock()
		return count == 1
	})

	cancelSender()

	select {
	case <-senderDone:
	case <-time.After(time.Second):
		t.Fatal("sender did not exit after cancellation")
	}

	select {
	case <-receiverDone:
	case <-time.After(time.Second):
		t.Fatal("receiver remained blocked after sender cancellation")
	}

	resp := receiverRec.Result()
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusGone {
		t.Fatalf("expected 410, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Sender disconnected before all receivers connected") {
		t.Fatalf("unexpected body: %q", string(body))
	}
}
