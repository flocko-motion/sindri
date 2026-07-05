// package: hub / agentserver
// type:    logic (per-agent socket = identity, Linux)
// job:     serve each agent's own unix socket (Linux). The socket a caller connects
//          through IS its (project, agent) identity — no name on the wire. Sockets
//          live under the central state dir. Exposes the agent surface: GET
//          /commands, GET /directive, POST /exec.
// limits:  agent sockets carry only the agent surface; host-control endpoints live
//          on the control socket (server.go). macOS uses the TCP channel instead
//          (agenttcp.go) — a bind-mounted socket can't cross the podman VM.
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

	"github.com/flo-at/sindri/internal/paths"
)

// ExecReq is the body for POST /exec on an agent socket.
type ExecReq struct {
	Args []string `json:"args"`
}

// AgentSocketDir is an agent's own socket directory under the central state dir,
// keyed by project (repoTag). The pod bind-mounts this DIRECTORY (not the socket
// file), so the agent keeps reaching the socket across hub restarts: a restart
// recreates the socket file (new inode), which a directory mount reflects.
func AgentSocketDir(project, name string) string {
	return filepath.Join(paths.StateDir(), project, "sockets", name)
}

// AgentSocketPath is the agent's socket file inside its socket directory.
func AgentSocketPath(project, name string) string {
	return filepath.Join(AgentSocketDir(project, name), "sock")
}

// ServeAgent starts (idempotently) the listener on an agent's socket. The socket
// file is (re)created here, so this must run before the agent's pod is launched
// (the pod bind-mounts its directory). Must run inside the persistent hub.
func (h *Hub) ServeAgent(project, name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := agentKey{project, name}
	if _, ok := h.agentLn[key]; ok {
		return nil
	}
	if err := os.MkdirAll(AgentSocketDir(project, name), 0o755); err != nil {
		return fmt.Errorf("agent socket dir %s/%s: %w", project, name, err)
	}
	path := AgentSocketPath(project, name)
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("serve agent socket %s/%s: %w", project, name, err)
	}
	h.agentLn[key] = ln
	go http.Serve(ln, logRequests(project+"/"+name, h.agentHandler(project, name)))
	return nil
}

// ServeAgents starts listeners for every rostered agent across all projects —
// called on hub boot so a restarted hub re-serves all agent sockets (D11).
func (h *Hub) ServeAgents() error {
	agents, err := h.store.AllAgents()
	if err != nil {
		return err
	}
	for _, a := range agents {
		if err := h.ServeAgent(a.Project, a.Name); err != nil {
			return err
		}
	}
	return nil
}

// closeAgents shuts every agent listener (called from Close).
func (h *Hub) closeAgents() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for key, ln := range h.agentLn {
		ln.Close()
		os.Remove(AgentSocketPath(key.project, key.name))
		delete(h.agentLn, key)
	}
}

// closeAgent shuts a single agent's listener and removes its socket (used when the
// agent is deleted).
func (h *Hub) closeAgent(project, name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := agentKey{project, name}
	if ln, ok := h.agentLn[key]; ok {
		ln.Close()
		delete(h.agentLn, key)
	}
	os.Remove(AgentSocketPath(project, name))
}

// agentHandler builds the agent-facing mux, bound to a fixed (project, agent).
func (h *Hub) agentHandler(project, name string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /commands", func(w http.ResponseWriter, r *http.Request) {
		cmds, err := h.AgentCommands(project, name)
		writeJSON(w, cmds, err)
	})
	mux.HandleFunc("GET /directive", func(w http.ResponseWriter, r *http.Request) {
		// Blocks until the agent has something to do (or it disconnects).
		d, err := h.AgentDirective(r.Context(), project, name)
		writeJSON(w, okMsg{d}, err)
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
		exit, err := h.AgentExec(project, name, req.Args, fw)
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
