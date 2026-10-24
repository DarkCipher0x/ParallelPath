package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"pp/server/internal/svc"
)

// Hub fans events out to connected devices. Events carry no content, only "something changed in space X", and
// devices then fetch and verify the new data themselves.
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan svc.Event]struct{} // person -> subscribers
}

// NewHub creates an empty hub.
func NewHub() *Hub { return &Hub{subs: map[string]map[chan svc.Event]struct{}{}} }

// Notify implements svc.Notifier. It never blocks: a slow device simply misses a nudge and catches up on its
// next fetch.
func (h *Hub) Notify(people []string, ev svc.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, p := range people {
		for ch := range h.subs[p] {
			select {
			case ch <- ev:
			default:
			}
		}
	}
}

func (h *Hub) subscribe(person string) chan svc.Event {
	ch := make(chan svc.Event, 64)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[person] == nil {
		h.subs[person] = map[chan svc.Event]struct{}{}
	}
	h.subs[person][ch] = struct{}{}
	return ch
}

func (h *Hub) unsubscribe(person string, ch chan svc.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs[person], ch)
	if len(h.subs[person]) == 0 {
		delete(h.subs, person)
	}
}

// events streams Server-Sent Events to a signed-in device until it disconnects.
func (a *Server) events(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	fl, okf := w.(http.Flusher)
	if !okf {
		fail(w, svc.ErrInvalid)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	ch := a.hub.subscribe(c.Person)
	defer a.hub.unsubscribe(c.Person, ch)
	rc := http.NewResponseController(w)
	_, _ = fmt.Fprint(w, "event: ready\ndata: {}\n\n")
	fl.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Second))
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			b, _ := json.Marshal(ev)
			if _, err := fmt.Fprintf(w, "event: change\ndata: %s\n\n", b); err != nil {
				return
			}
			fl.Flush()
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}
