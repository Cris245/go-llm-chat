package sse

import (
	"fmt"      
	"net/http" 
)

// Event represents a generic Server-Sent Event (SSE).
// It has a Type (e.g., "Status", "Message") and Data (the actual content).
type Event struct {
	Type string
	Data string
}

// Struct to manage SSE connections.
type Handler struct{}

// NewHandler creates and returns a new instance of SSEHandler.
func NewHandler() *Handler {
	return &Handler{}
}

// This function is called by the Go HTTP server when a request comes to our SSE path.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, eventChan <-chan Event) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported by this HTTP server", http.StatusInternalServerError)
		return
	}

	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: %s\n", event.Type)
			fmt.Fprintf(w, "data: %s\n\n", event.Data)
			flusher.Flush()
		case <-r.Context().Done():
			fmt.Println("Client disconnected.")
			return
		}
	}
}