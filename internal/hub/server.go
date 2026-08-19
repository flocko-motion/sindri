// package: hub / server
// type:    logic (HTTP/JSON over a unix socket)
// job:     expose the hub's operations as a small HTTP API on the repo's unix
// socket — GET /state, POST /agents, POST /launch, POST /tell. The
// single point every client (CLI, TUI, later agents) talks to.
// limits:  pure transport over Hub methods; no domain logic of its own.
package hub

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/server"
)

// AgentReq is the body for POST /agents; it crosses the wire, so it is
// internal/api.AgentReq under the name every existing caller here already uses.
type AgentReq = api.AgentReq

// TellReq is the body for POST /tell; it crosses the wire, so it is internal/api.TellReq
// under the name every existing caller here already uses.
type TellReq = api.TellReq

// PlanReq is the body for POST /agent/plan; it crosses the wire, so it is
// internal/api.PlanReq under the name every existing caller here already uses.
type PlanReq = api.PlanReq

// ChatSayReq is the body for POST /chat/say; it crosses the wire, so it is
// internal/api.ChatSayReq under the name every existing caller here already uses.
type ChatSayReq = api.ChatSayReq

// ChatView is the chatroom snapshot served by GET /chat and streamed by
// GET /chat/stream; it crosses the wire, so it is internal/api.ChatView under the
// name every existing caller here already uses.
type ChatView = api.ChatView

// NameReq is the body for operations addressing one agent (POST /launch) or PR
// (POST /merge); it crosses the wire, so it is internal/api.NameReq under the name
// every existing caller here already uses.
type NameReq = api.NameReq

// RunPriorityReq is the body for POST /run/priority; it crosses the wire, so it is
// internal/api.RunPriorityReq under the name every existing caller here already uses.
type RunPriorityReq = api.RunPriorityReq

// RepoReq targets a registered repo by its tag (POST /repo/forget, /repo/color); it
// crosses the wire, so it is internal/api.RepoReq under the name every existing
// caller here already uses.
type RepoReq = api.RepoReq

// Handler builds the HTTP mux over a hub. Every repo-scoped handler resolves its
// project from the request header (reqProject); the board endpoints scope tasks to
// the selected repo while keeping agents/PRs global.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		st, err := h.State(h.reqProject(r))
		writeJSON(w, st, err)
	})
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		report, err := h.Stats(r.Context())
		writeJSON(w, report, err)
	})
	mux.HandleFunc("GET /events", h.handleEvents)
	mux.HandleFunc("POST /refresh", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, okMsg{"refreshed"}, h.Refresh(h.reqProject(r)))
	})
	mux.HandleFunc("POST /tasks/reconcile", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, okMsg{"reconciled"}, h.wf.ReconcileTasks(h.reqProject(r)))
	})
	mux.HandleFunc("POST /task/comments/refresh", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the task id
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"refreshed"}, h.comments.Refresh(h.reqProject(r), req.Name))
	})
	// Comment on a task. An issue tracker that owns the thread receives it; everything else is
	// recorded here, where the thread lives (-> comments.Add).
	mux.HandleFunc("POST /task/comments/add", func(w http.ResponseWriter, r *http.Request) {
		var req TellReq // Name = the task id, Msg = the comment, Source = its author
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"commented"}, h.comments.Add(h.reqProject(r), req.Name, req.Source, req.Msg))
	})
	mux.HandleFunc("GET /repos", func(w http.ResponseWriter, r *http.Request) {
		list, err := h.projects.List()
		writeJSON(w, list, err)
	})
	mux.HandleFunc("GET /repo", func(w http.ResponseWriter, r *http.Request) {
		tag := r.URL.Query().Get("tag")
		if tag == "" {
			tag = h.reqProject(r) // default to the caller's repo
		}
		d, err := h.projects.Info(tag)
		writeJSON(w, d, err)
	})
	mux.HandleFunc("POST /repo/init", func(w http.ResponseWriter, r *http.Request) {
		sum, err := h.projects.Init(r.Header.Get("X-Sindri-Project")) // repo-scoped: header is the root
		writeJSON(w, sum, err)
	})
	mux.HandleFunc("POST /repo/forget", func(w http.ResponseWriter, r *http.Request) {
		var req RepoReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"forgotten"}, h.projects.Forget(req.Tag))
	})
	mux.HandleFunc("POST /repo/color", func(w http.ResponseWriter, r *http.Request) {
		var req RepoReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"ok"}, h.projects.SetColor(req.Tag, req.Color))
	})
	mux.HandleFunc("POST /orphan/remove", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the orphan container name
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"removed"}, h.projects.RemoveOrphan(detached(r), req.Name))
	})
	mux.HandleFunc("POST /repo/config", func(w http.ResponseWriter, r *http.Request) {
		var cfg config.Config
		if !decode(w, r, &cfg) {
			return
		}
		writeJSON(w, okMsg{"saved"}, h.projects.WriteConfig(r.Header.Get("X-Sindri-Project"), cfg))
	})
	mux.HandleFunc("GET /log", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("agent")
		evs, err := h.Log(h.agentReq(r, name), name)
		writeJSON(w, evs, err)
	})
	mux.HandleFunc("GET /agent/pane", func(w http.ResponseWriter, r *http.Request) {
		lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
		if lines <= 0 {
			lines = 40
		}
		name := r.URL.Query().Get("agent")
		out, err := h.agents.AgentPane(r.Context(), h.agentReq(r, name), name, lines)
		writeJSON(w, okMsg{out}, err)
	})
	mux.HandleFunc("GET /agent/pod", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("agent")
		out, err := h.agents.PodInfo(h.agentReq(r, name), name)
		writeJSON(w, okMsg{out}, err)
	})
	mux.HandleFunc("GET /agent/diagnose", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("agent")
		writeJSON(w, okMsg{h.agents.AgentDiagnostic(r.Context(), h.agentReq(r, name), name)}, nil)
	})
	mux.HandleFunc("GET /agent/clients", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("agent")
		cs, err := h.agents.Clients(r.Context(), h.agentReq(r, name), name)
		writeJSON(w, cs, err)
	})
	mux.HandleFunc("POST /agents", func(w http.ResponseWriter, r *http.Request) {
		var req AgentReq
		if !decode(w, r, &req) {
			return
		}
		name, err := h.agents.NewAgent(h.reqProject(r), req.Name, req.Role, req.Memory)
		writeJSON(w, okMsg{name}, err)
	})
	mux.HandleFunc("POST /agent/memory", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"ok"}, h.agents.SetMemory(h.agentReq(r, req.Name), req.Name, req.Memory))
	})
	mux.HandleFunc("POST /agent/retire", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"ok"}, h.SetRetired(h.agentReq(r, req.Name), req.Name, req.Retired))
	})
	// The user's own clear of an escalation. The agent normally clears its own (`sindri resume`), but
	// one that is gone, restarted, or simply wrong that it was blocked would stay stuck otherwise.
	mux.HandleFunc("POST /agent/resume", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"resumed"}, h.Resume(h.agentReq(r, req.Name), req.Name, "escalation cleared by the user"))
	})
	mux.HandleFunc("POST /agent/delete", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"deleted"}, h.agents.DeleteAgent(detached(r), h.agentReq(r, req.Name), req.Name))
	})
	mux.HandleFunc("POST /agent/stop", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"stopped"}, h.agents.StopAgent(detached(r), h.agentReq(r, req.Name), req.Name))
	})
	mux.HandleFunc("POST /agent/clear-context", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"ok"}, h.agents.SetClearArmed(detached(r), h.agentReq(r, req.Name), req.Name, req.Armed))
	})
	mux.HandleFunc("POST /agent/rebase", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"rebased"}, h.wf.RebaseAgent(h.agentReq(r, req.Name), req.Name))
	})
	mux.HandleFunc("POST /launch", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		// Stream build/start progress so the client isn't frozen during a long image
		// build; carry any error in a trailer (like /exec carries the exit code).
		w.Header().Set("Trailer", "X-Sindri-Error")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fw := &flushWriter{w: w}
		if f, ok := w.(http.Flusher); ok {
			fw.f = f
		}
		if err := h.agents.Launch(detached(r), h.agentReq(r, req.Name), req.Name, req.Shell, req.Debug, req.Cols, req.Lines, fw); err != nil {
			fmt.Fprintf(fw, "error: %v\n", err)
			w.Header().Set("X-Sindri-Error", err.Error())
		}
	})
	mux.HandleFunc("POST /agent/rebuild", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		// Same streaming shape as /launch — the image rebuild is long.
		w.Header().Set("Trailer", "X-Sindri-Error")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fw := &flushWriter{w: w}
		if f, ok := w.(http.Flusher); ok {
			fw.f = f
		}
		if err := h.agents.RebuildAgent(detached(r), h.agentReq(r, req.Name), req.Name, fw); err != nil {
			fmt.Fprintf(fw, "error: %v\n", err)
			w.Header().Set("X-Sindri-Error", err.Error())
		}
	})
	h.messageRoutes(mux)
	mux.HandleFunc("POST /chat/add", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"added"}, h.chat.Add(h.agentReq(r, req.Name), req.Name))
	})
	mux.HandleFunc("POST /chat/remove", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"removed"}, h.chat.Remove(h.agentReq(r, req.Name), req.Name))
	})
	mux.HandleFunc("POST /chat/say", func(w http.ResponseWriter, r *http.Request) {
		var req ChatSayReq
		if !decode(w, r, &req) {
			return
		}
		// A line starting with "/" is an in-chat command (add/remove/who/help),
		// executed by the hub; anything else is broadcast as the user.
		writeJSON(w, okMsg{"sent"}, h.chat.UserMessage(req.Msg))
	})
	mux.HandleFunc("POST /chat/new", func(w http.ResponseWriter, r *http.Request) {
		// Clears the shared history and announces it; membership survives.
		writeJSON(w, okMsg{"new meeting"}, h.chat.NewMeeting())
	})
	mux.HandleFunc("POST /chat/close", func(w http.ResponseWriter, r *http.Request) {
		_, err := h.chat.Close()
		writeJSON(w, okMsg{"meeting closed"}, err)
	})
	mux.HandleFunc("POST /chat/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		h.chat.Heartbeat()
		writeJSON(w, okMsg{"ok"}, nil)
	})
	mux.HandleFunc("GET /chat", func(w http.ResponseWriter, r *http.Request) {
		v, err := h.chatView()
		writeJSON(w, v, err)
	})
	mux.HandleFunc("GET /chat/stream", h.handleChatEvents)
	mux.HandleFunc("POST /merge", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the PR id
		if !decode(w, r, &req) {
			return
		}
		pr, err := h.wf.Merge(h.wf.PRProject(h.reqProject(r), req.Name), req.Name)
		writeJSON(w, pr, err)
	})
	mux.HandleFunc("POST /milestone", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the agent holding the container
		if !decode(w, r, &req) {
			return
		}
		pr, err := h.wf.MilestonePR(h.agentReq(r, req.Name), req.Name)
		writeJSON(w, pr, err)
	})
	mux.HandleFunc("GET /prs", func(w http.ResponseWriter, r *http.Request) {
		prs, err := h.wf.FleetPRs() // fleet-wide, matching the TUI board — not cwd-scoped
		writeJSON(w, prs, err)
	})
	mux.HandleFunc("GET /pr", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		d, err := h.wf.PRInfo(h.wf.PRProject(h.reqProject(r), id), id)
		writeJSON(w, d, err)
	})
	mux.HandleFunc("POST /pr/reject", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq
		if !decode(w, r, &req) {
			return
		}
		// The reject endpoint is the human path (TUI/CLI); resolve the PR fleet-wide.
		writeJSON(w, okMsg{"rejected"}, h.wf.RejectPR(h.wf.PRProject(h.reqProject(r), req.ID), req.ID, req.Feedback))
	})
	mux.HandleFunc("POST /pr/approve", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the PR id; the human approve path.
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"approved"}, h.wf.ApprovePR(h.wf.PRProject(h.reqProject(r), req.Name), req.Name))
	})
	mux.HandleFunc("POST /pr/scrap", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the PR id; the human scrap path (discard with its task).
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"scrapped"}, h.wf.ScrapPR(h.wf.PRProject(h.reqProject(r), req.Name), req.Name))
	})
	// Discarding a PR on its own is a DIFFERENT operation from scrapping one alongside its
	// task: with no close to free the author, this path has to release it (-> DiscardPR).
	mux.HandleFunc("POST /pr/discard", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"scrapped"}, h.wf.DiscardPR(h.wf.PRProject(h.reqProject(r), req.Name), req.Name))
	})
	mux.HandleFunc("GET /pr/lint", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		out, err := h.wf.LintPR(h.wf.PRProject(h.reqProject(r), id), id)
		writeJSON(w, okMsg{out}, err)
	})
	mux.HandleFunc("POST /pr/review", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq // reuse: ID + Feedback (the requirement text)
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"review requested"}, h.wf.RequestReview(h.wf.PRProject(h.reqProject(r), req.ID), req.ID, req.Feedback))
	})
	mux.HandleFunc("GET /review-prompt", func(w http.ResponseWriter, r *http.Request) {
		p, err := h.wf.ReviewPrompt(h.reqProject(r))
		writeJSON(w, okMsg{p}, err)
	})
	mux.HandleFunc("GET /pr/materialize", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		path, err := h.wf.MaterializeReview(h.wf.PRProject(h.reqProject(r), id), id)
		writeJSON(w, okMsg{path}, err)
	})
	h.runRoutes(mux) // the run queue's own surface (-> server_runs.go)
	mux.HandleFunc("GET /tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks, err := h.wf.Tasks(h.reqProject(r))
		writeJSON(w, tasks, err)
	})
	mux.HandleFunc("GET /task", func(w http.ResponseWriter, r *http.Request) {
		t, err := h.wf.TaskInfo(h.reqProject(r), r.URL.Query().Get("id"))
		writeJSON(w, t, err)
	})
	mux.HandleFunc("GET /task/next", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("agent")
		x, err := h.wf.ExplainNext(h.agentReq(r, name), name, r.URL.Query().Get("role"))
		writeJSON(w, x, err)
	})
	mux.HandleFunc("POST /tasks", func(w http.ResponseWriter, r *http.Request) {
		var req TaskReq
		if !decode(w, r, &req) {
			return
		}
		id, err := h.wf.CreateTask(h.reqProject(r), req.Spec())
		writeJSON(w, okMsg{id}, err)
	})
	mux.HandleFunc("POST /task/edit", func(w http.ResponseWriter, r *http.Request) {
		var req TaskReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{req.ID}, h.wf.EditTask(h.reqProject(r), req.ID, req.Spec()))
	})
	mux.HandleFunc("POST /priority", func(w http.ResponseWriter, r *http.Request) {
		var req PriorityReq
		if !decode(w, r, &req) {
			return
		}
		// An unrecognised scope is refused rather than narrowed: a caller that asked to rate a whole
		// tree and got one task would read the "ok" as having done it.
		scope, ok := api.ParsePriorityScope(req.Scope)
		if !ok {
			writeJSON(w, okMsg{"ok"}, fmt.Errorf("unknown priority scope %q (task, unrated, all)", req.Scope))
			return
		}
		writeJSON(w, okMsg{"ok"}, h.wf.SetPriority(h.reqProject(r), req.ID, req.Priority, scope))
	})
	// Assign a planner one thing to plan, as a phased brief (-> AssignPlan). Refused while that
	// planner has a PR open, so a new plan can't be drafted over specs still awaiting a verdict.
	mux.HandleFunc("POST /agent/plan", func(w http.ResponseWriter, r *http.Request) {
		var req PlanReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"assigned"}, h.wf.AssignPlan(h.agentReq(r, req.Name), req.Name, req.Goal, req.Task))
	})
	mux.HandleFunc("POST /task/approve", func(w http.ResponseWriter, r *http.Request) {
		var req ApproveTaskReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"approved"}, h.wf.ApproveTask(h.reqProject(r), req.ID, req.Subtree))
	})
	mux.HandleFunc("POST /task/reject", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq // ID + Feedback (the rejection comment)
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"rejected"}, h.wf.RejectTask(h.reqProject(r), req.ID, req.Feedback))
	})
	mux.HandleFunc("POST /task/unassign", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq // ID (+ unused Feedback)
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"unassigned"}, h.wf.UnassignTask(h.reqProject(r), req.ID))
	})
	mux.HandleFunc("POST /task/close", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq // ID (+ unused Feedback)
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"closed"}, h.wf.CloseTask(h.reqProject(r), req.ID))
	})
	// The host's counterpart to close: restores a closed sindri-owned task, with a reason
	// (-> Hub.ReopenTask, which also records it as a task comment).
	mux.HandleFunc("POST /task/reopen", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq // ID + Feedback (the reopen reason)
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"reopened"}, h.ReopenTask(h.reqProject(r), req.ID, api.SenderUser, req.Feedback))
	})
	mux.HandleFunc("POST /task/delete", func(w http.ResponseWriter, r *http.Request) {
		var req ScrapTaskReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"deleted"}, h.wf.ScrapTask(h.reqProject(r), req.ID, req.Subtree, req.PRs))
	})
	return mux
}

// PriorityReq is the body for POST /priority; it crosses the wire, so it is
// internal/api.PriorityReq under the name every existing caller here already uses.
type PriorityReq = api.PriorityReq

// RejectReq is the body for POST /pr/reject; it crosses the wire, so it is
// internal/api.RejectReq under the name every existing caller here already uses.
type RejectReq = api.RejectReq

// ApproveTaskReq is the body for POST /task/approve; it crosses the wire, so it is
// internal/api.ApproveTaskReq under the name every existing caller here already uses.
type ApproveTaskReq = api.ApproveTaskReq

// ScrapTaskReq is the body for POST /task/delete; it crosses the wire, so it is
// internal/api.ScrapTaskReq under the name every existing caller here already uses.
type ScrapTaskReq = api.ScrapTaskReq

// TaskReq is the body for POST /tasks (create) and POST /task/edit (ID set); it
// crosses the wire, so it is internal/api.TaskReq under the name every existing
// caller here already uses.
type TaskReq = api.TaskReq

// Serve binds the repo's unix socket and serves until the listener closes. A
// stale socket file from a previous run is removed first.
func (h *Hub) Serve() error {
	// Re-serve every rostered agent's socket so a restarted hub recovers all
	// agent channels (D11).
	if err := h.agentCh.ServeAgents(); err != nil {
		return err
	}
	// macOS: unix sockets can't cross the podman VM boundary, so also serve the
	// agent surface over a loopback TCP channel (token-authenticated).
	if runtime.GOOS == "darwin" {
		if err := h.agentCh.ServeTCP(); err != nil {
			return err
		}
	}
	h.wf.HealPlannerTasks()               // a planner can't hold a backlog task — release any stale claim
	h.wf.ReconcileMergingPRs()            // a merge in flight when we last died → merge-failed (outcome unknown)
	h.wf.ReconcileRunningRuns(h.lifetime) // a run in flight when we last died → failed (outcome unknown)
	// Seed each known project's task cache so its board is populated from the start.
	// A per-project failure (typically no td store at that repo) is not fatal — the
	// hub still serves agents/PRs — but it must be loud, not silent.
	known, kerr := h.projects.Known()
	if kerr != nil {
		fmt.Fprintf(os.Stderr, "hub: WARNING — could not read the repo registry: %v\n", kerr)
	}
	for _, p := range known {
		if err := h.wf.SyncTasks(p.Tag); err != nil {
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
	return http.Serve(ln, server.LogRequests("hub", requireProject(h.Handler())))
}
