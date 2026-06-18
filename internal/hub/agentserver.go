// package: hub / agentserver
// type:    logic (per-agent socket = identity)
// job:     serve each agent's own unix socket. The socket an agent connects
//
//	through IS its identity (D2): a connection on `.sindri/sockets/
//	<name>.sock` is, by construction, agent <name> — no name on the wire.
//	Exposes the agent surface: GET /commands, POST /exec (streamed).
//
// limits:  agent sockets carry only the agent surface; host-control endpoints
//
//	(/agents, /launch, /tell) live on the control socket (server.go).
package hub

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// ExecReq is the body for POST /exec on an agent socket.
type ExecReq struct {
	Args []string `json:"args"`
}

// AgentSocketPath is an agent's own socket — the one mounted into its pod.
func AgentSocketPath(root, name string) string {
	return filepath.Join(root, ".sindri", "sockets", name+".sock")
}

// ServeAgent starts (idempotently) the listener on an agent's socket. The
// socket file is (re)created here, so this must run before the agent's pod is
// launched (the pod bind-mounts it). Must run inside the persistent hub.
func (h *Hub) ServeAgent(name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.agentLn[name]; ok {
		return nil
	}
	path := AgentSocketPath(h.root, name)
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("serve agent socket %s: %w", name, err)
	}
	h.agentLn[name] = ln
	go http.Serve(ln, logRequests(name, h.agentHandler(name)))
	return nil
}

// ServeAgents starts listeners for every rostered agent — called on hub boot so
// a restarted hub re-serves all agent sockets (D11).
func (h *Hub) ServeAgents() error {
	roster, err := h.store.Roster()
	if err != nil {
		return err
	}
	for _, a := range roster {
		if err := h.ServeAgent(a.Name); err != nil {
			return err
		}
	}
	return nil
}

// closeAgents shuts every agent listener (called from Close).
func (h *Hub) closeAgents() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for name, ln := range h.agentLn {
		ln.Close()
		os.Remove(AgentSocketPath(h.root, name))
		delete(h.agentLn, name)
	}
}

// closeAgent shuts a single agent's listener and removes its socket (used when
// the agent is deleted).
func (h *Hub) closeAgent(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ln, ok := h.agentLn[name]; ok {
		ln.Close()
		delete(h.agentLn, name)
	}
	os.Remove(AgentSocketPath(h.root, name))
}

// agentHandler builds the agent-facing mux, bound to a fixed caller identity.
func (h *Hub) agentHandler(name string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /commands", func(w http.ResponseWriter, r *http.Request) {
		cmds, err := h.AgentCommands(name)
		writeJSON(w, cmds, err)
	})
	mux.HandleFunc("POST /exec", func(w http.ResponseWriter, r *http.Request) {
		var req ExecReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, nil, err)
			return
		}
		w.Header().Set("Trailer", "X-Sindri-Exit")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fw := &flushWriter{w: w}
		if f, ok := w.(http.Flusher); ok {
			fw.f = f
		}
		exit, err := h.AgentExec(name, req.Args, fw)
		if err != nil {
			fmt.Fprintf(fw, "error: %v\n", err)
			if exit == 0 {
				exit = 1
			}
		}
		w.Header().Set("X-Sindri-Exit", strconv.Itoa(exit))
	})
	return mux
}

// flushWriter flushes after every write so command output streams to the client.
type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if fw.f != nil {
		fw.f.Flush()
	}
	return n, err
}
