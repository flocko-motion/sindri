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

// NameReq is the body for operations addressing one agent (POST /launch).
type NameReq struct {
	Name string `json:"name"`
}

// Handler builds the HTTP mux over a hub.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		st, err := h.State()
		writeJSON(w, st, err)
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
		writeJSON(w, okMsg{"launched"}, h.Launch(req.Name))
	})
	mux.HandleFunc("POST /tell", func(w http.ResponseWriter, r *http.Request) {
		var req TellReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"delivered"}, h.Tell(req.Name, req.Msg, req.Source))
	})
	return mux
}

// Serve binds the repo's unix socket and serves until the listener closes. A
// stale socket file from a previous run is removed first.
func (h *Hub) Serve() error {
	// Re-serve every rostered agent's socket so a restarted hub recovers all
	// agent channels (D11).
	if err := h.ServeAgents(); err != nil {
		return err
	}
	path := h.SocketPath()
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	return http.Serve(ln, h.Handler())
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
