package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

const version = "0.1.0"

// router dispatches requests to reserved path handlers or the piping server.
type router struct {
	piping *PipingServer
}

func (rt *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS headers on all responses
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Disposition, X-Piping")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.Header().Set("X-Robots-Tag", "none")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	path := r.URL.Path

	// Reserved paths (GET only)
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		switch path {
		case "/":
			handleIndex(w, r)
			return
		case "/version":
			handleVersion(w, r)
			return
		case "/help":
			handleHelp(w, r)
			return
		case "/favicon.ico":
			handleFavicon(w, r)
			return
		case "/robots.txt":
			handleRobots(w, r)
			return
		case "/app.js":
			handleStatic("app.js", "application/javascript")(w, r)
			return
		case "/style.css":
			handleStatic("style.css", "text/css")(w, r)
			return
		}
	}

	// Piping paths
	switch r.Method {
	case http.MethodPost, http.MethodPut:
		rt.piping.HandleSender(w, r)
	case http.MethodGet:
		rt.piping.HandleReceiver(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8888"
	}

	ps := NewPipingServer()
	rt := &router{piping: ps}

	fmt.Printf("piping-go %s listening on :%s\n", version, port)
	log.Fatal(http.ListenAndServe(":"+port, rt))
}
