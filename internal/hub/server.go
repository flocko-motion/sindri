// package: hub / server
// type:    logic (HTTP/JSON over a unix socket)
// job:     expose the hub's operations as a small HTTP API on the repo's unix
//
//	socket — GET /state, POST /agents, POST /launch, POST /tell. The
//	single point every client (CLI, TUI, later agents) talks to.
//
// limits:  pure transport over Hub methods; no domain logic of its own.
package hub

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
)

// AgentReq is the body for POST /agents.
type AgentReq struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// TellReq is the body for POST /tell.
type TellReq struct {
	Name   string `json:"name"`
	Msg    string `json:"msg"`
	Source string `json:"source"`
}

// NameReq is the body for operations addressing one agent (POST /launch) or PR
// (POST /merge). Shell applies to /launch only.
type NameReq struct {
	Name  string `json:"name"`
	Shell bool   `json:"shell"`
}

// Handler builds the HTTP mux over a hub.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		st, err := h.State()
		writeJSON(w, st, err)
	})
	mux.HandleFunc("GET /events", h.handleEvents)
	mux.HandleFunc("POST /refresh", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, okMsg{"refreshed"}, h.Refresh())
	})
	mux.HandleFunc("GET /log", func(w http.ResponseWriter, r *http.Request) {
		evs, err := h.Log(r.URL.Query().Get("agent"))
		writeJSON(w, evs, err)
	})
	mux.HandleFunc("POST /agents", func(w http.ResponseWriter, r *http.Request) {
		var req AgentReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"registered"}, h.NewAgent(req.Name, req.Role))
	})
	mux.HandleFunc("POST /launch", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"launched"}, h.Launch(req.Name, req.Shell))
	})
	mux.HandleFunc("POST /tell", func(w http.ResponseWriter, r *http.Request) {
		var req TellReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"delivered"}, h.Tell(req.Name, req.Msg, req.Source))
	})
	mux.HandleFunc("POST /merge", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the PR id
		if !decode(w, r, &req) {
			return
		}
		pr, err := h.Merge(req.Name)
		writeJSON(w, pr, err)
	})
	mux.HandleFunc("GET /prs", func(w http.ResponseWriter, r *http.Request) {
		prs, err := h.PRs()
		writeJSON(w, prs, err)
	})
	mux.HandleFunc("GET /pr", func(w http.ResponseWriter, r *http.Request) {
		d, err := h.PRInfo(r.URL.Query().Get("id"))
		writeJSON(w, d, err)
	})
	mux.HandleFunc("GET /tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks, err := h.Tasks()
		writeJSON(w, tasks, err)
	})
	mux.HandleFunc("GET /task", func(w http.ResponseWriter, r *http.Request) {
		t, err := h.TaskInfo(r.URL.Query().Get("id"))
		writeJSON(w, t, err)
	})
	mux.HandleFunc("POST /tasks", func(w http.ResponseWriter, r *http.Request) {
		var req TaskReq
		if !decode(w, r, &req) {
			return
		}
		id, err := h.NewTask(req.Title, req.Type, req.Priority, req.Labels)
		writeJSON(w, okMsg{id}, err)
	})
	mux.HandleFunc("POST /priority", func(w http.ResponseWriter, r *http.Request) {
		var req PriorityReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"ok"}, h.SetPriority(req.ID, req.Priority))
	})
	return mux
}

// PriorityReq is the body for POST /priority.
type PriorityReq struct {
	ID       string `json:"id"`
	Priority string `json:"priority"`
}

// TaskReq is the body for POST /tasks.
type TaskReq struct {
	Title    string   `json:"title"`
	Type     string   `json:"type"`
	Priority string   `json:"priority"`
	Labels   []string `json:"labels"`
}

// Serve binds the repo's unix socket and serves until the listener closes. A
// stale socket file from a previous run is removed first.
func (h *Hub) Serve() error {
	// Re-serve every rostered agent's socket so a restarted hub recovers all
	// agent channels (D11).
	if err := h.ServeAgents(); err != nil {
		return err
	}
	_ = h.SyncTasks() // seed the task cache so the board is populated from the start
	path := h.SocketPath()
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	return http.Serve(ln, h.Handler())
}

// handleEvents streams board state as Server-Sent Events: the current state on
// connect, then a fresh snapshot on every change, until the client disconnects.
func (h *Hub) handleEvents(w http.ResponseWriter, r *http.Request) {
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

	send := func() {
		st, err := h.State()
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

type okMsg struct {
	OK string `json:"ok"`
}

type errMsg struct {
	Error string `json:"error"`
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSON(w, nil, err)
		return false
	}
	return true
}

// writeJSON writes v as JSON, or a 400 with the error message if err != nil.
func writeJSON(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(errMsg{err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}
