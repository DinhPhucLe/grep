package orgknowledge

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Hub fans out newly created knowledge documents to org-scoped SSE subscribers.
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan Document]struct{}
}

// NewHub creates an empty pub/sub hub.
func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[chan Document]struct{})}
}

// Subscribe receives documents for organizationID. Call cancel to unsubscribe.
func (h *Hub) Subscribe(organizationID string) (<-chan Document, func()) {
	ch := make(chan Document, 8)
	h.mu.Lock()
	if h.subs[organizationID] == nil {
		h.subs[organizationID] = make(map[chan Document]struct{})
	}
	h.subs[organizationID][ch] = struct{}{}
	h.mu.Unlock()
	cancel := func() {
		h.mu.Lock()
		if set, ok := h.subs[organizationID]; ok {
			delete(set, ch)
			if len(set) == 0 {
				delete(h.subs, organizationID)
			}
		}
		h.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

// Publish notifies subscribers for doc.OrganizationID. Non-blocking per subscriber.
func (h *Hub) Publish(doc Document) {
	if h == nil {
		return
	}
	h.mu.Lock()
	set := h.subs[doc.OrganizationID]
	targets := make([]chan Document, 0, len(set))
	for ch := range set {
		targets = append(targets, ch)
	}
	h.mu.Unlock()
	for _, ch := range targets {
		select {
		case ch <- doc:
		default:
		}
	}
}

// NewEventsHandler streams knowledge.created SSE events for the caller's org.
// Expects auth.Principal on the request context (via auth.Require).
func NewEventsHandler(hub *Hub) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
			return
		}
		principal, ok := principalFrom(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "stream_unsupported", "streaming unsupported")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		ch, cancel := hub.Subscribe(principal.OrganizationID.Hex())
		defer cancel()

		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-heartbeat.C:
				_, _ = w.Write([]byte(": ping\n\n"))
				flusher.Flush()
			case doc, open := <-ch:
				if !open {
					return
				}
				payload, err := json.Marshal(map[string]any{
					"type": "knowledge.created",
					"item": doc,
				})
				if err != nil {
					continue
				}
				_, _ = w.Write([]byte("event: knowledge.created\ndata: "))
				_, _ = w.Write(payload)
				_, _ = w.Write([]byte("\n\n"))
				flusher.Flush()
			}
		}
	})
}
