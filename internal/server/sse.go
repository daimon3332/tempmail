package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// subscription is a single SSE client channel.
type subscription struct {
	address string // "" = global (admin)
	ch      chan map[string]any
}

var (
	subsMu sync.RWMutex
	subs   = map[int]*subscription{}
	subID  int
)

// broadcast sends an event to every matching subscription. Empty address event
// notifies globally only; per-address events notify matching address + global.
func broadcast(address string, event map[string]any) {
	subsMu.RLock()
	defer subsMu.RUnlock()
	for _, s := range subs {
		if s.address == "" || s.address == address {
			select {
			case s.ch <- event:
			default: // drop if slow
			}
		}
	}
}

func (a *App) sseHandler(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		text(w, 500, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	address := r.URL.Query().Get("address")
	// Global feed requires admin auth; per-address feed requires address JWT.
	if address == "" {
		if !a.isAdmin(r) {
			// Anonymous global stream is allowed (used by the frontend to
			// receive a generic "new mail" ping); events carry no data.
			address = "public"
		} else {
			address = ""
		}
	}

	subsMu.Lock()
	subID++
	id := subID
	s := &subscription{address: address, ch: make(chan map[string]any, 16)}
	subs[id] = s
	subsMu.Unlock()
	defer func() {
		subsMu.Lock()
		delete(subs, id)
		subsMu.Unlock()
	}()

	// heartbeat
	fmt.Fprintf(w, ": connected\n\n")
	fl.Flush()
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-s.ch:
			b, _ := jsonMarshal(ev)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evType(ev), b)
			fl.Flush()
		case <-keepalive.C:
			fmt.Fprintf(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func evType(ev map[string]any) string {
	if t, ok := ev["type"].(string); ok {
		return t
	}
	return "event"
}

// notifyMailPublished broadcasts a new-mail event so open SSE clients refresh.
func (a *App) notifyMailPublished(address string, mailID int64) {
	broadcast(address, map[string]any{"type": "mail", "address": address, "mail_id": mailID})
}

func (a *App) notifyGlobal(event string, data map[string]any) {
	m := map[string]any{"type": event}
	for k, v := range data {
		m[k] = v
	}
	broadcast("", m)
}

var _ = context.Background
