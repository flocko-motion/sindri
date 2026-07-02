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
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"
)

// quietPaths are high-frequency reads (UI polling) excluded from the access log
// so it stays an action log, not a flood.
var quietPaths = map[string]bool{"/state": true, "/events": true, "/log": true, "/agent/pane": true, "/agent/pod": true}

// logRequests wraps a handler to print one access-log line per request — the
// hub's window onto every action it executes. label is the socket's owner
// ("hub" or an agent name).
func logRequests(label string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if quietPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		log.Printf("%-8s %-4s %-14s %d %s", label, r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

// statusRecorder captures the response status while passing flushing through
// (needed for the streamed /exec and SSE endpoints).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

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

// reqProject resolves (and registers) the repo a host request concerns, from the
// X-Sindri-Project header (the client sends its repo root). Returns the repoTag; ""
// when no header is present (a repo-agnostic request, e.g. the board with no repo
// selected). This is the single place a host request's project is derived.
func (h *Hub) reqProject(r *http.Request) string {
	root := r.Header.Get("X-Sindri-Project")
	if root == "" {
		return ""
	}
	h.repo(root) // register (idempotent) + ensure .worktrees gitignore
	return repoTag(root)
}

// Handler builds the HTTP mux over a hub. Every repo-scoped handler resolves its
// project from the request header (reqProject); the board endpoints scope tasks to
// the selected repo while keeping agents/PRs global.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		st, err := h.State(h.reqProject(r))
		writeJSON(w, st, err)
	})
	mux.HandleFunc("GET /events", h.handleEvents)
	mux.HandleFunc("POST /refresh", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, okMsg{"refreshed"}, h.Refresh(h.reqProject(r)))
	})
	mux.HandleFunc("GET /log", func(w http.ResponseWriter, r *http.Request) {
		evs, err := h.Log(h.reqProject(r), r.URL.Query().Get("agent"))
		writeJSON(w, evs, err)
	})
	mux.HandleFunc("GET /agent/pane", func(w http.ResponseWriter, r *http.Request) {
		lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
		if lines <= 0 {
			lines = 40
		}
		out, err := h.AgentPane(h.reqProject(r), r.URL.Query().Get("agent"), lines)
		writeJSON(w, okMsg{out}, err)
	})
	mux.HandleFunc("GET /agent/pod", func(w http.ResponseWriter, r *http.Request) {
		out, err := h.PodInfo(h.reqProject(r), r.URL.Query().Get("agent"))
		writeJSON(w, okMsg{out}, err)
	})
	mux.HandleFunc("POST /agents", func(w http.ResponseWriter, r *http.Request) {
		var req AgentReq
		if !decode(w, r, &req) {
			return
		}
		name, err := h.NewAgent(h.reqProject(r), req.Name, req.Role)
		writeJSON(w, okMsg{name}, err)
	})
	mux.HandleFunc("POST /agent/delete", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"deleted"}, h.DeleteAgent(h.reqProject(r), req.Name))
	})
	mux.HandleFunc("POST /agent/stop", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"stopped"}, h.StopAgent(h.reqProject(r), req.Name))
	})
	mux.HandleFunc("POST /launch", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"launched"}, h.Launch(h.reqProject(r), req.Name, req.Shell))
	})
	mux.HandleFunc("POST /tell", func(w http.ResponseWriter, r *http.Request) {
		var req TellReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"delivered"}, h.Tell(h.reqProject(r), req.Name, req.Msg, req.Source))
	})
	mux.HandleFunc("POST /merge", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the PR id
		if !decode(w, r, &req) {
			return
		}
		pr, err := h.Merge(h.reqProject(r), req.Name)
		writeJSON(w, pr, err)
	})
	mux.HandleFunc("POST /milestone", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the agent holding the container
		if !decode(w, r, &req) {
			return
		}
		pr, err := h.MilestonePR(h.reqProject(r), req.Name)
		writeJSON(w, pr, err)
	})
	mux.HandleFunc("GET /prs", func(w http.ResponseWriter, r *http.Request) {
		prs, err := h.PRs(h.reqProject(r))
		writeJSON(w, prs, err)
	})
	mux.HandleFunc("GET /pr", func(w http.ResponseWriter, r *http.Request) {
		d, err := h.PRInfo(h.reqProject(r), r.URL.Query().Get("id"))
		writeJSON(w, d, err)
	})
	mux.HandleFunc("POST /pr/reject", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq
		if !decode(w, r, &req) {
			return
		}
		// The reject endpoint is the human path (TUI/CLI).
		writeJSON(w, okMsg{"rejected"}, h.RejectPR(h.reqProject(r), req.ID, req.Feedback))
	})
	mux.HandleFunc("POST /pr/approve", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the PR id; the human approve path.
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"approved"}, h.ApprovePR(h.reqProject(r), req.Name))
	})
	mux.HandleFunc("GET /pr/lint", func(w http.ResponseWriter, r *http.Request) {
		out, err := h.LintPR(h.reqProject(r), r.URL.Query().Get("id"))
		writeJSON(w, okMsg{out}, err)
	})
	mux.HandleFunc("POST /pr/review", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq // reuse: ID + Feedback (the requirement text)
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"review requested"}, h.RequestReview(h.reqProject(r), req.ID, req.Feedback))
	})
	mux.HandleFunc("GET /review-prompt", func(w http.ResponseWriter, r *http.Request) {
		p, err := h.ReviewPrompt(h.reqProject(r))
		writeJSON(w, okMsg{p}, err)
	})
	mux.HandleFunc("GET /pr/materialize", func(w http.ResponseWriter, r *http.Request) {
		path, err := h.MaterializeReview(h.reqProject(r), r.URL.Query().Get("id"))
		writeJSON(w, okMsg{path}, err)
	})
	mux.HandleFunc("GET /tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks, err := h.Tasks(h.reqProject(r))
		writeJSON(w, tasks, err)
	})
	mux.HandleFunc("GET /task", func(w http.ResponseWriter, r *http.Request) {
		t, err := h.TaskInfo(h.reqProject(r), r.URL.Query().Get("id"))
		writeJSON(w, t, err)
	})
	mux.HandleFunc("POST /tasks", func(w http.ResponseWriter, r *http.Request) {
		var req TaskReq
		if !decode(w, r, &req) {
			return
		}
		id, err := h.CreateTask(h.reqProject(r), req.spec())
		writeJSON(w, okMsg{id}, err)
	})
	mux.HandleFunc("POST /task/edit", func(w http.ResponseWriter, r *http.Request) {
		var req TaskReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{req.ID}, h.EditTask(h.reqProject(r), req.ID, req.spec()))
	})
	mux.HandleFunc("POST /priority", func(w http.ResponseWriter, r *http.Request) {
		var req PriorityReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"ok"}, h.SetPriority(h.reqProject(r), req.ID, req.Priority))
	})
	mux.HandleFunc("POST /task/approve", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq // reuse: ID (+ unused Feedback)
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"approved"}, h.ApproveTask(h.reqProject(r), req.ID))
	})
	mux.HandleFunc("POST /task/reject", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq // ID + Feedback (the rejection comment)
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"rejected"}, h.RejectTask(h.reqProject(r), req.ID, req.Feedback))
	})
	mux.HandleFunc("POST /task/unassign", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq // ID (+ unused Feedback)
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"unassigned"}, h.UnassignTask(h.reqProject(r), req.ID))
	})
	return mux
}

// PriorityReq is the body for POST /priority.
type PriorityReq struct {
	ID       string `json:"id"`
	Priority string `json:"priority"`
}

// RejectReq is the body for POST /pr/reject.
type RejectReq struct {
	ID       string `json:"id"`
	Feedback string `json:"feedback"`
}

// TaskReq is the body for POST /tasks (create) and POST /task/edit (ID set).
type TaskReq struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Priority    string   `json:"priority"`
	Parent      string   `json:"parent"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
}

func (r TaskReq) spec() TaskSpec {
	return TaskSpec{Title: r.Title, Type: r.Type, Priority: r.Priority, Parent: r.Parent, Description: r.Description, Labels: r.Labels}
}

// Serve binds the repo's unix socket and serves until the listener closes. A
// stale socket file from a previous run is removed first.
func (h *Hub) Serve() error {
	// Re-serve every rostered agent's socket so a restarted hub recovers all
	// agent channels (D11).
	if err := h.ServeAgents(); err != nil {
		return err
	}
	// macOS: unix sockets can't cross the podman VM boundary, so also serve the
	// agent surface over a loopback TCP channel (token-authenticated).
	if runtime.GOOS == "darwin" {
		if err := h.serveAgentTCP(); err != nil {
			return err
		}
	}
	h.healPlannerTasks() // a planner can't hold a backlog task — release any stale claim
	// Seed each known project's task cache so its board is populated from the start.
	// A per-project failure (typically no td store at that repo) is not fatal — the
	// hub still serves agents/PRs — but it must be loud, not silent.
	for _, p := range h.knownProjects() {
		if err := h.SyncTasks(p.Tag); err != nil {
			fmt.Fprintf(os.Stderr, "hub: WARNING — could not load tasks for %s: %v\n", p.Path, err)
		}
	}
	path := h.SocketPath()
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	return http.Serve(ln, logRequests("hub", h.Handler()))
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

	project := h.reqProject(r) // the selected repo scopes the board's tasks
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
