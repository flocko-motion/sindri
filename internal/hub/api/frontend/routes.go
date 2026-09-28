// package: hub/api/frontend / routes
// type:    logic (HTTP/JSON over the hub's control socket)
// job:     the front-end surface — one route per operation, for the host CLI and the TUI.
// Every repo-scoped route resolves its project from the request header
// (-> Hub.ReqProject); the board routes keep agents and PRs global.
// limits:  transport over the hub's handles; no rule of its own lives here, and the
// agents' own surface is served elsewhere (-> hub/api/agents).
package frontend

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/api/serve"
	"github.com/flo-at/sindri/internal/hub/harness"
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
func Handler(h Hub) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		st, err := h.State(h.ReqProject(r))
		serve.WriteJSON(w, st, err)
	})
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		report, err := h.Stats(r.Context())
		serve.WriteJSON(w, report, err)
	})
	mux.HandleFunc("GET /events", h.HandleEvents)
	mux.HandleFunc("POST /refresh", func(w http.ResponseWriter, r *http.Request) {
		serve.WriteJSON(w, serve.OKMsg{"refreshed"}, h.Refresh(h.ReqProject(r)))
	})
	mux.HandleFunc("POST /tasks/reconcile", func(w http.ResponseWriter, r *http.Request) {
		serve.WriteJSON(w, serve.OKMsg{"reconciled"}, h.TaskFlow().ReconcileTasks(h.ReqProject(r)))
	})
	mux.HandleFunc("POST /task/comments/refresh", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the task id
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"refreshed"}, h.Comments().Refresh(h.ReqProject(r), req.Name))
	})
	// Comment on a task. An issue tracker that owns the thread receives it; everything else is
	// recorded here, where the thread lives (-> comments.Add).
	mux.HandleFunc("POST /task/comments/add", func(w http.ResponseWriter, r *http.Request) {
		var req TellReq // Name = the task id, Msg = the comment, Source = its author
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"commented"}, h.Comments().Add(h.ReqProject(r), req.Name, req.Source, req.Msg))
	})
	mux.HandleFunc("GET /repos", func(w http.ResponseWriter, r *http.Request) {
		list, err := h.Projects().List()
		serve.WriteJSON(w, list, err)
	})
	mux.HandleFunc("GET /repo", func(w http.ResponseWriter, r *http.Request) {
		tag := r.URL.Query().Get("tag")
		if tag == "" {
			tag = h.ReqProject(r) // default to the caller's repo
		}
		d, err := h.Projects().Info(tag)
		serve.WriteJSON(w, d, err)
	})
	mux.HandleFunc("POST /repo/init", func(w http.ResponseWriter, r *http.Request) {
		sum, err := h.Projects().Init(r.Header.Get("X-Sindri-Project")) // repo-scoped: header is the root
		serve.WriteJSON(w, sum, err)
	})
	mux.HandleFunc("POST /repo/forget", func(w http.ResponseWriter, r *http.Request) {
		var req RepoReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"forgotten"}, h.Projects().Forget(req.Tag))
	})
	mux.HandleFunc("POST /repo/color", func(w http.ResponseWriter, r *http.Request) {
		var req RepoReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"ok"}, h.Projects().SetColor(req.Tag, req.Color))
	})
	mux.HandleFunc("POST /orphan/remove", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the orphan container name
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"removed"}, h.Projects().RemoveOrphan(serve.Detached(r), req.Name))
	})
	mux.HandleFunc("POST /repo/config", func(w http.ResponseWriter, r *http.Request) {
		var cfg config.Config
		if !serve.Decode(w, r, &cfg) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"saved"}, h.Projects().WriteConfig(r.Header.Get("X-Sindri-Project"), cfg))
	})
	// /activity, not /log: `log` is the agent's verb for WRITING a note on its own work, and one
	// name meaning a write on one surface and a read on the other is a trap for both readers.
	mux.HandleFunc("GET /activity", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("agent")
		evs, err := h.Log(h.AgentReq(r, name), name)
		serve.WriteJSON(w, evs, err)
	})
	mux.HandleFunc("GET /agent/states", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("agent")
		evs, err := h.StateLog(h.AgentReq(r, name), name)
		serve.WriteJSON(w, evs, err)
	})
	// Starts the flow debug view's own loopback listener, which carries its reads (-> debugview).
	mux.HandleFunc("POST /debug/serve", func(w http.ResponseWriter, r *http.Request) {
		var req api.DebugServeReq
		if !serve.Decode(w, r, &req) {
			return
		}
		url, err := h.DebugServe(req.Port)
		serve.WriteJSON(w, api.DebugServeResp{URL: url}, err)
	})
	mux.HandleFunc("GET /agent/pane", func(w http.ResponseWriter, r *http.Request) {
		lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
		if lines <= 0 {
			lines = 40
		}
		name := r.URL.Query().Get("agent")
		out, err := h.Agents().AgentPane(r.Context(), h.AgentReq(r, name), name, lines)
		serve.WriteJSON(w, serve.OKMsg{out}, err)
	})
	mux.HandleFunc("GET /agent/pod", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("agent")
		out, err := h.Agents().PodInfo(h.AgentReq(r, name), name)
		serve.WriteJSON(w, serve.OKMsg{out}, err)
	})
	mux.HandleFunc("GET /agent/diagnose", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("agent")
		serve.WriteJSON(w, serve.OKMsg{h.Agents().AgentDiagnostic(r.Context(), h.AgentReq(r, name), name)}, nil)
	})
	mux.HandleFunc("GET /agent/clients", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("agent")
		cs, err := h.Agents().Clients(r.Context(), h.AgentReq(r, name), name)
		serve.WriteJSON(w, cs, err)
	})
	mux.HandleFunc("POST /agents", func(w http.ResponseWriter, r *http.Request) {
		var req AgentReq
		if !serve.Decode(w, r, &req) {
			return
		}
		name, err := h.Agents().NewAgent(h.ReqProject(r), req.Name, req.Role, req.Memory)
		serve.WriteJSON(w, serve.OKMsg{name}, err)
	})
	mux.HandleFunc("POST /agent/memory", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"ok"}, h.Agents().SetMemory(h.AgentReq(r, req.Name), req.Name, req.Memory))
	})
	mux.HandleFunc("POST /agent/retire", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"ok"}, h.SetRetired(h.AgentReq(r, req.Name), req.Name, req.Retired))
	})
	// The user's own clear of an escalation. The agent normally clears its own (`sindri resume`), but
	// one that is gone, restarted, or simply wrong that it was blocked would stay stuck otherwise.
	mux.HandleFunc("POST /agent/resume", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"resumed"}, h.AgentFlow().ResumeByUser(h.AgentReq(r, req.Name), req.Name, req.Answer))
	})
	mux.HandleFunc("POST /agent/delete", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"deleted"}, h.Agents().DeleteAgent(serve.Detached(r), h.AgentReq(r, req.Name), req.Name))
	})
	mux.HandleFunc("POST /agent/stop", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !serve.Decode(w, r, &req) {
			return
		}
		// The request, not the act: the machine's stopping state takes the pod back, and settling the
		// agent here means this answers after it has rather than before.
		project := h.AgentReq(r, req.Name)
		err := h.AgentFlow().AskStop(project, req.Name)
		if err == nil {
			h.AgentFlow().Settle(project, req.Name) // the pod is gone before this answers, not after
		}
		serve.WriteJSON(w, serve.OKMsg{"stopped"}, err)
	})
	mux.HandleFunc("POST /agent/clear-context", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"ok"}, h.Agents().SetClearArmed(serve.Detached(r), h.AgentReq(r, req.Name), req.Name, req.Armed))
	})
	mux.HandleFunc("POST /agent/rebase", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"rebased"}, h.PRFlow().RebaseAgent(h.AgentReq(r, req.Name), req.Name))
	})
	mux.HandleFunc("POST /launch", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !serve.Decode(w, r, &req) {
			return
		}
		// Stream build/start progress so the client isn't frozen during a long image
		// build; carry any error in a trailer (like /exec carries the exit code).
		w.Header().Set("Trailer", "X-Sindri-Error")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fw := serve.Flushing(w)
		fail := func(err error) {
			fmt.Fprintf(fw, "error: %v\n", err)
			w.Header().Set("X-Sindri-Error", err.Error())
		}
		// The request, not the act, as with stop: the machine's launching state starts the pod, and
		// settling the agent here answers after it has. The ask carries this caller's options into
		// that launch and streams its output back, so an image build is still watched live.
		project := h.AgentReq(r, req.Name)
		ask := h.Agents().AskLaunch(project, req.Name,
			harness.LaunchOpts{Shell: req.Shell, Debug: req.Debug, Cols: req.Cols, Lines: req.Lines}, fw)
		defer h.Agents().Withdraw(project, req.Name, ask)
		if err := h.AgentFlow().AskStart(project, req.Name); err != nil {
			fail(err)
			return
		}
		h.AgentFlow().Settle(project, req.Name)
		if ran, err := ask.Result(); ran {
			if err != nil {
				fail(err)
			}
			return
		}
		// No launch took the request: the pod was already up, or where the agent stands does not
		// answer a start. The second is said, and the request taken back rather than left to fire later.
		if h.Agents().AgentAlive(serve.Detached(r), project, req.Name) {
			fmt.Fprintf(fw, "%s is already running.\n", req.Name)
			return
		}
		h.AgentFlow().AnswerPodRequest(project, req.Name)
		fail(fmt.Errorf("%s was not started: it stands in %s, which does not answer a start",
			req.Name, h.Fleet().Standing(project, req.Name).State))
	})
	mux.HandleFunc("POST /agent/rebuild", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !serve.Decode(w, r, &req) {
			return
		}
		// Same streaming shape as /launch — the image rebuild is long.
		w.Header().Set("Trailer", "X-Sindri-Error")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fw := serve.Flushing(w)
		if err := h.Agents().RebuildAgent(serve.Detached(r), h.AgentReq(r, req.Name), req.Name, fw); err != nil {
			fmt.Fprintf(fw, "error: %v\n", err)
			w.Header().Set("X-Sindri-Error", err.Error())
		}
	})
	messageRoutes(mux, h)
	mux.HandleFunc("POST /chat/add", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"added"}, h.Chat().Add(h.AgentReq(r, req.Name), req.Name))
	})
	mux.HandleFunc("POST /chat/remove", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"removed"}, h.Chat().Remove(h.AgentReq(r, req.Name), req.Name))
	})
	mux.HandleFunc("POST /chat/say", func(w http.ResponseWriter, r *http.Request) {
		var req ChatSayReq
		if !serve.Decode(w, r, &req) {
			return
		}
		// A line starting with "/" is an in-chat command (add/remove/who/help),
		// executed by the hub; anything else is broadcast as the user.
		serve.WriteJSON(w, serve.OKMsg{"sent"}, h.Chat().UserMessage(req.Msg))
	})
	mux.HandleFunc("POST /chat/new", func(w http.ResponseWriter, r *http.Request) {
		// Clears the shared history and announces it; membership survives.
		serve.WriteJSON(w, serve.OKMsg{"new meeting"}, h.Chat().NewMeeting())
	})
	mux.HandleFunc("POST /chat/close", func(w http.ResponseWriter, r *http.Request) {
		_, err := h.Chat().Close()
		serve.WriteJSON(w, serve.OKMsg{"meeting closed"}, err)
	})
	mux.HandleFunc("POST /chat/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		h.Chat().Heartbeat()
		serve.WriteJSON(w, serve.OKMsg{"ok"}, nil)
	})
	mux.HandleFunc("GET /chat", func(w http.ResponseWriter, r *http.Request) {
		v, err := h.ChatView()
		serve.WriteJSON(w, v, err)
	})
	mux.HandleFunc("GET /chat/stream", h.HandleChatEvents)
	mux.HandleFunc("POST /merge", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the PR id
		if !serve.Decode(w, r, &req) {
			return
		}
		pr, err := h.PRFlow().Merge(h.PRFlow().PRProject(h.ReqProject(r), req.Name), req.Name)
		serve.WriteJSON(w, pr, err)
	})
	mux.HandleFunc("POST /milestone", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the agent holding the container
		if !serve.Decode(w, r, &req) {
			return
		}
		pr, err := h.PRFlow().MilestonePR(h.AgentReq(r, req.Name), req.Name)
		serve.WriteJSON(w, pr, err)
	})
	mux.HandleFunc("GET /prs", func(w http.ResponseWriter, r *http.Request) {
		prs, err := h.PRFlow().FleetPRs() // fleet-wide, matching the TUI board — not cwd-scoped
		serve.WriteJSON(w, prs, err)
	})
	mux.HandleFunc("GET /pr", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		d, err := h.PRFlow().PRInfo(h.PRFlow().PRProject(h.ReqProject(r), id), id)
		serve.WriteJSON(w, d, err)
	})
	mux.HandleFunc("POST /pr/reject", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq
		if !serve.Decode(w, r, &req) {
			return
		}
		// The reject endpoint is the human path (TUI/CLI); resolve the PR fleet-wide.
		serve.WriteJSON(w, serve.OKMsg{"rejected"}, h.PRFlow().RejectPR(h.PRFlow().PRProject(h.ReqProject(r), req.ID), req.ID, req.Feedback))
	})
	mux.HandleFunc("POST /pr/approve", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the PR id; the human approve path.
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"approved"}, h.PRFlow().ApprovePR(h.PRFlow().PRProject(h.ReqProject(r), req.Name), req.Name))
	})
	mux.HandleFunc("POST /pr/scrap", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the PR id; the human scrap path (discard with its task).
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"scrapped"}, h.PRFlow().ScrapPR(h.PRFlow().PRProject(h.ReqProject(r), req.Name), req.Name))
	})
	// Discarding a PR on its own is a DIFFERENT operation from scrapping one alongside its
	// task: with no close to free the author, this path has to release it (-> DiscardPR).
	mux.HandleFunc("POST /pr/discard", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"scrapped"}, h.PRFlow().DiscardPR(h.PRFlow().PRProject(h.ReqProject(r), req.Name), req.Name))
	})
	mux.HandleFunc("GET /pr/lint", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		out, err := h.PRFlow().LintPR(h.PRFlow().PRProject(h.ReqProject(r), id), id)
		serve.WriteJSON(w, serve.OKMsg{out}, err)
	})
	mux.HandleFunc("POST /pr/review", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq // reuse: ID + Feedback (the requirement text)
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"review requested"}, h.PRFlow().RequestReview(h.PRFlow().PRProject(h.ReqProject(r), req.ID), req.ID, req.Feedback))
	})
	mux.HandleFunc("GET /review-prompt", func(w http.ResponseWriter, r *http.Request) {
		p, err := h.PRFlow().ReviewPrompt(h.ReqProject(r))
		serve.WriteJSON(w, serve.OKMsg{p}, err)
	})
	mux.HandleFunc("GET /pr/materialize", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		path, err := h.PRFlow().MaterializeReview(h.PRFlow().PRProject(h.ReqProject(r), id), id)
		serve.WriteJSON(w, serve.OKMsg{path}, err)
	})
	runRoutes(mux, h) // the run queue's own surface (-> runs.go)
	mux.HandleFunc("GET /tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks, err := h.TaskFlow().Tasks(h.ReqProject(r))
		serve.WriteJSON(w, tasks, err)
	})
	mux.HandleFunc("GET /task", func(w http.ResponseWriter, r *http.Request) {
		t, err := h.TaskFlow().TaskInfo(h.ReqProject(r), r.URL.Query().Get("id"))
		serve.WriteJSON(w, t, err)
	})
	mux.HandleFunc("GET /task/next", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("agent")
		x, err := h.Fleet().ExplainNext(h.AgentReq(r, name), name, r.URL.Query().Get("role"))
		serve.WriteJSON(w, x, err)
	})
	mux.HandleFunc("POST /tasks", func(w http.ResponseWriter, r *http.Request) {
		var req TaskReq
		if !serve.Decode(w, r, &req) {
			return
		}
		id, err := h.TaskFlow().CreateTask(h.ReqProject(r), req.Spec())
		serve.WriteJSON(w, serve.OKMsg{id}, err)
	})
	mux.HandleFunc("POST /task/edit", func(w http.ResponseWriter, r *http.Request) {
		var req TaskReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{req.ID}, h.TaskFlow().EditTask(h.ReqProject(r), req.ID, req.Spec()))
	})
	mux.HandleFunc("POST /priority", func(w http.ResponseWriter, r *http.Request) {
		var req PriorityReq
		if !serve.Decode(w, r, &req) {
			return
		}
		// An unrecognised scope is refused rather than narrowed: a caller that asked to rate a whole
		// tree and got one task would read the "ok" as having done it.
		scope, ok := api.ParsePriorityScope(req.Scope)
		if !ok {
			serve.WriteJSON(w, serve.OKMsg{"ok"}, fmt.Errorf("unknown priority scope %q (task, unrated, all)", req.Scope))
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"ok"}, h.TaskFlow().SetPriority(h.ReqProject(r), req.ID, req.Priority, scope))
	})
	// Assign a planner one thing to plan, as a phased brief (-> AssignPlan). Refused while that
	// planner has a PR open, so a new plan can't be drafted over specs still awaiting a verdict.
	mux.HandleFunc("POST /agent/plan", func(w http.ResponseWriter, r *http.Request) {
		var req PlanReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"assigned"}, h.TaskFlow().AssignPlan(h.AgentReq(r, req.Name), req.Name, req.Goal, req.Task))
	})
	mux.HandleFunc("POST /task/approve", func(w http.ResponseWriter, r *http.Request) {
		var req ApproveTaskReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"approved"}, h.TaskFlow().ApproveTask(h.ReqProject(r), req.ID, req.Subtree))
	})
	mux.HandleFunc("POST /task/reject", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq // ID + Feedback (the rejection comment)
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"rejected"}, h.TaskFlow().RejectTask(h.ReqProject(r), req.ID, req.Feedback))
	})
	mux.HandleFunc("POST /task/unassign", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq // ID (+ unused Feedback)
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"unassigned"}, h.TaskFlow().UnassignTask(h.ReqProject(r), req.ID))
	})
	mux.HandleFunc("POST /task/close", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq // ID (+ unused Feedback)
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"closed"}, h.PRFlow().CloseTask(h.ReqProject(r), req.ID))
	})
	// The host's counterpart to close: restores a closed sindri-owned task, with a reason
	// (-> Hub.ReopenTask, which also records it as a task comment).
	mux.HandleFunc("POST /task/reopen", func(w http.ResponseWriter, r *http.Request) {
		var req RejectReq // ID + Feedback (the reopen reason)
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"reopened"}, h.ReopenTask(h.ReqProject(r), req.ID, api.SenderUser, req.Feedback))
	})
	mux.HandleFunc("POST /task/delete", func(w http.ResponseWriter, r *http.Request) {
		var req ScrapTaskReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"deleted"}, h.PRFlow().ScrapTask(h.ReqProject(r), req.ID, req.Subtree, req.PRs))
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
