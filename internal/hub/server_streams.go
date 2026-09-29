// package: hub / server_streams
// type:    adapter (Server-Sent Events)
// job:     the two endpoints that stay open — the board and the chatroom, each sending a
// snapshot on connect and a fresh one on every change until the client goes away.
// They have their own shape: no request body, no single response, and a lifetime
// bounded by the client rather than by the handler.
// limits:  transport only; assembling a snapshot is the read model's (-> State, ChatView).
package hub

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/flo-at/sindri/internal/api"
)

// HandleEvents streams board state as Server-Sent Events: the current state on
// connect, then a fresh snapshot on every change, until the client disconnects.
func (h *Hub) HandleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher.Flush() // send headers immediately so the client connects even if a
	// snapshot can't be built yet — never leave the request hanging.

	ch, unsub := h.events.subscribe()
	defer unsub()

	project := h.ReqProject(r) // the selected repo scopes the board's tasks
	send := func() {
		st, err := h.State(project)
		if err != nil {
			return
		}
		data, err := json.Marshal(st)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
	send() // initial snapshot
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			send()
		}
	}
}

// ChatView builds the current chatroom snapshot (roster + recent transcript).
func (h *Hub) ChatView() (api.ChatView, error) {
	members, err := h.chat.Members()
	if err != nil {
		return api.ChatView{}, err
	}
	log, err := h.chat.Transcript(0)
	if err != nil {
		return api.ChatView{}, err
	}
	return api.ChatView{Members: members, Log: log}, nil
}

// HandleChatEvents streams the chatroom as Server-Sent Events: the snapshot on
// connect, then a fresh one on every board change (chat included), until the
// client disconnects. This is how the user's live views (the `chat join` CLI and
// the TUI chat tab) receive forwarded messages — the star topology's user leg.
func (h *Hub) HandleChatEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher.Flush()

	ch, unsub := h.events.subscribe()
	defer unsub()

	send := func() {
		v, err := h.ChatView()
		if err != nil {
			return
		}
		data, err := json.Marshal(v)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
	send()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			send()
		}
	}
}
