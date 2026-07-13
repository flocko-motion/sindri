// package: hub/agentchan / agentchan
// type:    logic (the inbound agent command channel)
// job:     serve the agent surface (GET /commands, GET /directive, POST /exec) to
//          each agent — on Linux via its own unix socket (the socket path IS the
//          agent's identity), on macOS via a token-authenticated TCP channel (tcp.go).
//          Owns the listener lifecycle: serve at boot / launch, close on shutdown.
// limits:  transport only; the surface's behaviour comes from the hub via Deps, and
//          host-control endpoints live on the control socket (hub/server.go).
package agentchan

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/tools/paths"
)

// Deps is what the channel needs from the hub to answer the agent surface: the verb
// set, the blocking directive, verb execution, token→identity resolution, and the
// access-log wrapper. The channel owns transport; the behaviour stays in the hub.
type Deps interface {
	Commands(project, name string) (any, error)
	Directive(ctx context.Context, project, name string) (string, error)
	Exec(project, name string, args []string, out io.Writer) (int, error)
	TokenAgent(token string) (project, name string, ok bool, err error)
	LogRequests(label string, next http.Handler) http.Handler
}

// Server serves the agent command channel and owns its listener state.
type Server struct {
	store *store.Store
	deps  Deps

	mu   sync.Mutex               // guards unix
	unix map[agentKey]net.Listener // per-agent unix listeners (Linux)

	tcpLn    net.Listener // macOS TCP channel
	tcpPort  int
	dialHost string
}

// agentKey identifies an agent within a project (a listener-map key).
type agentKey struct{ project, name string }

// New builds the agent-channel server over the hub's store + Deps.
func New(st *store.Store, deps Deps) *Server {
	return &Server{store: st, deps: deps, unix: map[agentKey]net.Listener{}}
}

// ExecReq is the body for POST /exec on an agent channel.
type ExecReq struct {
	Args []string `json:"args"`
}

// SocketDir is an agent's own socket directory under the central state dir, keyed by
// project (repoTag). The pod bind-mounts this DIRECTORY (not the socket file), so the
// agent keeps reaching the socket across hub restarts: a restart recreates the socket
// file (new inode), which a directory mount reflects.
func SocketDir(project, name string) string {
	return filepath.Join(paths.StateDir(), project, "sockets", name)
}

// SocketPath is the agent's socket file inside its socket directory.
func SocketPath(project, name string) string {
	return filepath.Join(SocketDir(project, name), "sock")
}

// ServeAgent starts (idempotently) the listener on an agent's socket. The socket file
// is (re)created here, so this must run before the agent's pod is launched (the pod
// bind-mounts its directory). Must run inside the persistent hub.
func (s *Server) ServeAgent(project, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := agentKey{project, name}
	if _, ok := s.unix[k]; ok {
		return nil
	}
	if err := os.MkdirAll(SocketDir(project, name), 0o755); err != nil {
		return fmt.Errorf("agent socket dir %s/%s: %w", project, name, err)
	}
	path := SocketPath(project, name)
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("serve agent socket %s/%s: %w", project, name, err)
	}
	s.unix[k] = ln
	go http.Serve(ln, s.deps.LogRequests(project+"/"+name, s.handler(project, name)))
	return nil
}

// ServeAgents starts listeners for every rostered agent across all projects — called
// on hub boot so a restarted hub re-serves all agent sockets (D11).
func (s *Server) ServeAgents() error {
	agents, err := s.store.AllAgents()
	if err != nil {
		return err
	}
	for _, a := range agents {
		if err := s.ServeAgent(a.Project, a.Name); err != nil {
			return err
		}
	}
	return nil
}

// CloseAll shuts every listener — the per-agent unix sockets and the TCP channel —
// called from the hub's Close.
func (s *Server) CloseAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, ln := range s.unix {
		ln.Close()
		os.Remove(SocketPath(k.project, k.name))
		delete(s.unix, k)
	}
	if s.tcpLn != nil {
		s.tcpLn.Close()
	}
}

// CloseAgent shuts a single agent's listener and removes its socket (used when the
// agent is deleted).
func (s *Server) CloseAgent(project, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := agentKey{project, name}
	if ln, ok := s.unix[k]; ok {
		ln.Close()
		delete(s.unix, k)
	}
	os.Remove(SocketPath(project, name))
}

// handler builds the agent-facing mux, bound to a fixed (project, agent).
func (s *Server) handler(project, name string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /commands", func(w http.ResponseWriter, r *http.Request) {
		cmds, err := s.deps.Commands(project, name)
		writeJSON(w, cmds, err)
	})
	mux.HandleFunc("GET /directive", func(w http.ResponseWriter, r *http.Request) {
		// Blocks until the agent has something to do (or it disconnects).
		d, err := s.deps.Directive(r.Context(), project, name)
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
		exit, err := s.deps.Exec(project, name, req.Args, fw)
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

type okMsg struct {
	OK string `json:"ok"`
}

type errMsg struct {
	Error string `json:"error"`
}

// writeJSON writes v as JSON, or a 400 with the error message if err != nil — the
// same wire shape the control socket uses, so one client decoder handles both.
func writeJSON(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(errMsg{err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}
