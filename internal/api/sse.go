package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// Event is pushed to all subscribers of a project's SSE channel: file-tree
// changes, suggestion/comment activity, compile status, version creation.
type Event struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan []byte]struct{} // projectID -> subscribers
}

func NewHub() *Hub {
	return &Hub{subs: map[string]map[chan []byte]struct{}{}}
}

func (h *Hub) Publish(projectID string, ev Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[projectID] {
		select {
		case ch <- data:
		default: // drop for slow consumers; SSE clients refetch via REST
		}
	}
}

func (h *Hub) subscribe(projectID string) chan []byte {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	if h.subs[projectID] == nil {
		h.subs[projectID] = map[chan []byte]struct{}{}
	}
	h.subs[projectID][ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsubscribe(projectID string, ch chan []byte) {
	h.mu.Lock()
	delete(h.subs[projectID], ch)
	if len(h.subs[projectID]) == 0 {
		delete(h.subs, projectID)
	}
	h.mu.Unlock()
}

// ServeSSE streams project events; caller must have verified access.
func (h *Hub) ServeSSE(w http.ResponseWriter, r *http.Request, projectID string) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := h.subscribe(projectID)
	defer h.unsubscribe(projectID, ch)
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case data := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", data)
			fl.Flush()
		}
	}
}
