package server

import (
	"encoding/json"
	"sync"
)

// Hub fans run events out to SSE subscribers.
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan []byte]struct{}
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{subs: map[string]map[chan []byte]struct{}{}}
}

// Subscribe returns a buffered channel for one run.
func (h *Hub) Subscribe(runID string) chan []byte {
	ch := make(chan []byte, 256)
	h.mu.Lock()
	if h.subs[runID] == nil {
		h.subs[runID] = map[chan []byte]struct{}{}
	}
	h.subs[runID][ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber.
func (h *Hub) Unsubscribe(runID string, ch chan []byte) {
	h.mu.Lock()
	if subs := h.subs[runID]; subs != nil {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(h.subs, runID)
		}
	}
	h.mu.Unlock()
}

// Broadcast sends a JSON frame to every subscriber of a run.
func (h *Hub) Broadcast(runID string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[runID] {
		select {
		case ch <- data:
		default:
			// Slow client: drop this frame rather than block the agent.
		}
	}
}

// SubscriberCount reports active subscribers for diagnostics.
func (h *Hub) SubscriberCount(runID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[runID])
}
