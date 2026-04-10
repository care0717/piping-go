package main

import (
	"embed"
	"fmt"
	"net/http"
)

//go:embed web/index.html web/style.css web/app.js
var webFS embed.FS

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		// This shouldn't happen with Go 1.22+ routing, but guard anyway
		http.NotFound(w, r)
		return
	}
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, version)
}

func handleHelp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, `piping-go - Transfer data between devices via HTTP

Send:
  curl -T myfile.txt http://localhost:8888/mysecret
  cat data | curl -T - http://localhost:8888/mysecret

Receive:
  curl http://localhost:8888/mysecret > myfile.txt

Send to multiple receivers:
  curl -T myfile.txt "http://localhost:8888/mysecret?n=3"

Web UI:
  Open http://localhost:8888/ in your browser
`)
}

func handleFavicon(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func handleRobots(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func handleStatic(filename, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := webFS.ReadFile("web/" + filename)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(data)
	}
}
