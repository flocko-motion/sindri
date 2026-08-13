// package: hub / commands
// type:    logic (the hub-side verb set the browser invokes)
// job:     build the command registry with hub-bound behaviour, resolve a
// caller's identity/role/state, and execute a verb on its behalf
// (logging the socket call to the activity log). Phase 2 ships the
// mechanism with real `status`/`log` plus role-scoped Phase-3 stubs.
// limits:  workflow verbs (submit/next/approve/reject) gain real behaviour in
// Phase 3; here they exist only to prove surface filtering.
package hub

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/repo"
	"github.com/flo-at/sindri/internal/hub/task"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// CmdInfo is a command as advertised to a browser; it crosses the wire, so it is
// internal/api.CmdInfo under the name every existing caller here already uses.
type CmdInfo = api.CmdInfo

// registry builds the command surface, rebuilt per call; Run closures capture the hub.
func (h *Hub) registry() *registry.Registry {
	return registry.New(
		registry.Command{Name: "status", Help: "show who you are and your current state", Run: h.cmdStatus},
		registry.Command{Name: "log", Help: "record a note in your activity log: log <message>", Run: h.cmdLog},
		registry.Command{Name: "prs", Help: "list pull requests and their status", Run: h.cmdListPRs},
		registry.Command{Name: "show", Help: "show a PR's diff: show <pr-id>", Run: h.wf.CmdShowPR},
		// Only a worker grabs tasks and submits a branch. A planner has neither: it ships openspec
		// via its own `openspec submit`, a PR in different dress (mock todo id os-new).
		registry.Command{Name: "next", Help: "pick up the next task", Roles: []string{"worker"},
			Blocked: heldByEscalation("next", func(c registry.Caller) string {
				if c.HasTask {
					return "You already hold work — run `sindri` to be told what to do with it."
				}
				return ""
			}), Run: h.wf.CmdNext},
		registry.Command{Name: "lint", Help: "run the quality gate: lint (your workspace) or lint <pr-id> (a PR)", Run: h.cmdLint},
		// Visibility MUST match these commands' own `st.Phase != "working"` guard. When it didn't, a
		// worker in "submitted" was offered submit, ran it, and was told to abandon the task it held.
		registry.Command{Name: "submit", Help: "request your branch be merged: submit [message]", Roles: []string{"worker"},
			Blocked: heldByEscalation("submit", landingBlocked("submit")), Run: h.wf.CmdSubmit},
		// Land interim work mid-task without finishing it; same visibility as submit, task stays open.
		registry.Command{Name: "contribute", Help: "land an interim contribution mid-task (needs the user's approval): contribute [message]", Roles: []string{"worker"},
			Blocked: heldByEscalation("contribute", landingBlocked("contribute")), Run: h.wf.CmdContribute},
		// The author's own reject: it withdraws its PR to keep working. Available exactly while one is
		// out, which is the state where realising something is missing had no way out but somebody
		// else's verdict.
		registry.Command{Name: "revoke", Help: "withdraw your pull request and keep working on it: revoke [why]",
			Roles: []string{"worker", "planner"},
			Blocked: func(c registry.Caller) string {
				if c.Phase != "submitted" {
					return "You have no pull request out to withdraw — `sindri` tells you where you are."
				}
				return ""
			}, Run: h.wf.CmdRevoke},
		// Always available to a worker — checking your branch still merges is harmless at any time.
		registry.Command{Name: "resolve", Help: "check your branch still merges onto its base, and resolve any conflicts: resolve", Roles: []string{"worker"}, Run: h.wf.CmdResolve},
		// Align any time — harmless, and it surfaces conflicts to fix rather than letting drift.
		registry.Command{Name: "rebase", Help: "rebase your branch onto the current reference branch (fix any conflicts it surfaces): rebase", Roles: []string{"worker", "planner"}, Run: h.wf.CmdRebase},
		// The agents have no git of their own — that isolation is the point. So the hub runs a
		// curated read/restore subset for them (-> workflow.CmdGit): without it, an agent cannot
		// see what it changed or put a file back, and reconstructs both from memory.
		registry.Command{Name: "git", Help: workflow.GitHelp, Roles: []string{"worker", "planner", "coauthor"}, Run: h.wf.CmdGit},
		registry.Command{Name: "checkpoint", Help: "record the current subtask and move to the next: checkpoint [summary]", Roles: []string{"worker"},
			Blocked: heldByEscalation("checkpoint", func(c registry.Caller) string {
				if c.Container == "" {
					return "Checkpoint records one subtask of a feature, and you hold a task of your own — " +
						"`sindri submit \"<summary>\"` puts it up for review when it's done."
				}
				return ""
			}), Run: h.wf.CmdCheckpoint},
		// A worker reads too: it holds a whole package for context, so that context must stay
		// re-readable. Roles see different scopes (-> CmdTasks) but share one verb name.
		registry.Command{Name: "task", Help: workflow.TaskHelp, Roles: []string{"planner", "coauthor", "worker", "reviewer"}, Run: h.wf.CmdTasks},
		registry.Command{Name: "create-task", Help: workflow.CreateTaskHelp, Roles: []string{"planner"}, Run: h.wf.CmdCreateTask},
		registry.Command{Name: "edit-task", Help: workflow.EditTaskHelp, Roles: []string{"planner"}, Run: h.wf.CmdEditTask},
		registry.Command{Name: "prioritise-task", Help: workflow.PrioritiseTaskHelp, Roles: []string{"planner"}, Run: h.wf.CmdPrioritiseTask},
		// Planner only, never a worker, which could undo a human's verdict on its own task
		// (-> h.ReopenTask, which needs both h.wf and h.comments, so it lives here, not workflow).
		registry.Command{Name: "reopen-task", Help: reopenTaskHelp, Roles: []string{"planner"}, Run: h.cmdReopenTask},
		// A planner's landing verb: `openspec submit` is a worker's submit in different dress, so the
		// escalation hold has to reach it. Held open, an escalated planner could ship a PR built on the
		// guess it had just said it would not make.
		registry.Command{Name: "openspec", Help: "ship your openspec changes as a PR: openspec submit [message]", Roles: []string{"planner"},
			Blocked: heldByEscalation("openspec", nil), Run: h.wf.CmdOpenspec},
		registry.Command{Name: "state", Help: "set your resting state: state planning | state idle", Roles: []string{"planner"}, Run: h.wf.CmdState},
		// Scoped to what the role already sees (-> cmdComment): a worker its own task or held
		// container, a reviewer the task of the PR it's reviewing, a planner/coauthor any task —
		// they already read the whole backlog. Findings belong on the task, not the activity log.
		registry.Command{Name: "comment", Help: commentHelp(registry.Caller{}), HelpFor: commentHelp,
			Roles: []string{"worker", "reviewer", "planner", "coauthor"},
			Blocked: func(c registry.Caller) string {
				switch c.Role {
				case "worker":
					if !c.HasTask {
						return "You hold no task to comment on."
					}
				case "reviewer":
					// A store fault must not read as the settled "nothing to review" — err != nil
					// leaves the verb unblocked so cmdComment hits ReviewingPR again and returns the
					// error properly, rather than the agent being told a false reason to stop.
					if pr, err := h.store.For(c.Project).ReviewingPR(c.Agent); err == nil && pr == "" {
						return "You aren't reviewing a PR, so there's no task to comment on."
					}
				}
				return ""
			}, Run: h.cmdComment},
		// A planner's approve is a different act (-> CmdApprove): an optional, additional badge
		// beside the reviewer's verdict, never a substitute for it — so the same verb is open to
		// both roles rather than needing a second one.
		registry.Command{Name: "approve", Help: approveHelp(registry.Caller{}), HelpFor: approveHelp,
			Roles: []string{"reviewer", "planner"}, Blocked: heldByEscalation("approve", nil), Run: h.wf.CmdApprove},
		registry.Command{Name: "reject", Help: "reject a pull request: reject <pr-id> <feedback...>", Roles: []string{"reviewer"},
			Blocked: heldByEscalation("reject", nil), Run: h.wf.CmdReject},
		// Either side of a stop only the user can end, open to every role — any agent can meet a
		// decision that is not its to make. Escalate stays open while escalated: a badly-put
		// question has to be re-puttable.
		registry.Command{Name: "escalate", Help: escalateHelp, Run: h.cmdEscalate},
		registry.Command{Name: "resume", Help: resumeHelp,
			Blocked: func(c registry.Caller) string {
				if c.Escalation == "" {
					return "You have no escalation to clear — `sindri` tells you where you are."
				}
				return ""
			}, Run: h.cmdResume},
		// State-gated rather than role-gated: the user controls who is in the meeting room.
		registry.Command{Name: "meeting", Help: "say something to everyone in the meeting room: meeting <message...>",
			Blocked: func(c registry.Caller) string {
				if !c.InChat {
					return "You're not in the meeting room — the user adds agents to it, so there's nobody to say this to yet."
				}
				return ""
			}, Run: h.cmdChat},
	)
}

// landingBlocked is the shared gate on the two verbs that put a branch up (submit, contribute).
// Inside a feature submit is the FINISHED branch, so it waits for the last subtask, while contribute
// puts up what stands — a milestone, which needs nothing finished. Outside one, there is nothing to
// land except from "working", worded exactly as the verb's own guard words it so an agent hears one
// story whichever gate it meets first.
func landingBlocked(verb string) func(registry.Caller) string {
	return func(c registry.Caller) string {
		if c.Container != "" {
			switch {
			case verb == "contribute" && c.Phase == "submitted":
				return fmt.Sprintf("Feature %s is already up for the user to merge — wait for that, or "+
					"`sindri revoke` to take it back and keep working.", c.Container)
			case verb == "submit" && c.SubtasksOpen:
				return fmt.Sprintf("Feature %s still has open subtasks, and it goes up as ONE PR — "+
					"record the one you're on with `sindri checkpoint \"<summary>\"` and it will hand you "+
					"the next. Submit once they're all done; `sindri contribute` puts the branch up "+
					"meanwhile if what's on it is already useful.", c.Container)
			}
			return ""
		}
		if c.Phase != "working" {
			return workflow.ReplyNotWorking(verb, c.Phase, c.Task)
		}
		return ""
	}
}

// caller resolves an agent's identity and role within its project.
func (h *Hub) caller(project, name string) (registry.Caller, error) {
	ps := h.store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return registry.Caller{}, err
	}
	if !ok {
		return registry.Caller{}, fmt.Errorf("unknown agent %q", name)
	}
	// Holding a task or a collaborative container hides "next" and shows "submit" (a container
	// swaps in "checkpoint"); an idle worker gets the reverse.
	st, err := ps.GetState(name)
	if err != nil {
		return registry.Caller{}, err
	}
	// Chatroom membership gates the "chat" verb — invisible until the user adds this agent.
	inChat, err := h.store.ChatIsMember(project, name)
	if err != nil {
		return registry.Caller{}, err
	}
	// Whether the held feature has subtasks left is what decides submit, so it is read here rather
	// than inferred from the phase: after a rejected feature PR the worker is back to "working" with
	// nothing left to check point, and a phase proxy would have blocked the resubmit.
	subtasksOpen := false
	if st.Container != "" {
		open, oerr := ps.OpenSubtasks(st.Container)
		if oerr != nil {
			return registry.Caller{}, oerr
		}
		subtasksOpen = len(open) > 0
	}
	return registry.Caller{
		Project:      project,
		Agent:        name,
		Role:         a.Role,
		HasTask:      st.Phase != "idle" || st.Container != "",
		Container:    st.Container,
		SubtasksOpen: subtasksOpen,
		Task:         st.Task,
		Phase:        st.Phase,
		InChat:       inChat,
		Escalation:   st.Escalation,
	}, nil
}

// isHelpArg reports whether an argument asks for the verb's own help, in the spellings tried.
func isHelpArg(s string) bool {
	switch s {
	case "--help", "-h", "help", "-help":
		return true
	}
	return false
}

// AgentCommands returns an agent's whole role surface: what it can run now, and what it cannot
// along with why. A blocked verb is listed rather than dropped — an agent reads this as the set of
// things that EXIST, so omitting one it was told to run reads as a broken hub.
func (h *Hub) AgentCommands(project, name string) ([]CmdInfo, error) {
	c, err := h.caller(project, name)
	if err != nil {
		return nil, err
	}
	surface := h.registry().Surface(c)
	out := make([]CmdInfo, len(surface))
	for i, o := range surface {
		out[i] = CmdInfo{Name: o.Name, Help: o.HelpText(c), Unavailable: o.Blocked}
	}
	return out, nil
}

// AgentExec runs a verb for an agent, streaming to out and returning a process-style exit code.
func (h *Hub) AgentExec(project, name string, args []string, out io.Writer) (int, error) {
	c, err := h.caller(project, name)
	if err != nil {
		return 1, err
	}
	if len(args) == 0 {
		return 1, fmt.Errorf("no command given")
	}
	// Invocations aren't logged as activity — the meaningful ones record their own outcome
	// (claim/submit/note/approve/reject/merged), and reads aren't activity at all.
	cmd, blocked, ok := h.registry().Resolve(args[0], c)
	if !ok {
		// Name what IS available: a bare "unknown command" costs a turn per guess (a `sindri commit`
		// that never existed got tried twice). Available() gates as Resolve does, so nothing leaks.
		avail := h.registry().Available(c)
		names := make([]string, len(avail))
		for i, a := range avail {
			names[i] = a.Name
		}
		// A real verb held back by the state machine gets its reason and its replacement, rather than
		// the vanishing act that left an agent to reverse-engineer the workflow from an empty list.
		if blocked != "" {
			fmt.Fprintf(out, "`sindri %s` isn't available right now.\n%s\navailable now: %s\n",
				args[0], blocked, strings.Join(names, " "))
			return 1, nil
		}
		fmt.Fprintf(out, "unknown or unavailable command: %s\navailable now: %s\n", args[0], strings.Join(names, " "))
		return 127, nil
	}
	// `<verb> --help` comes from the registry, handled before Run so a help request reaches the
	// agent as help rather than as an argument the verb tries to interpret.
	if len(args) > 1 && isHelpArg(args[1]) {
		fmt.Fprintf(out, "%s\n", cmd.HelpText(c))
		return 0, nil
	}
	exit, err := cmd.Run(c, args[1:], out)
	h.notify() // the command may have changed board state
	if err != nil {
		// A returned error is a hub-INTERNAL failure (agent-actionable outcomes go to `out` with a nil
		// error) and may carry host paths or operator instructions — log it host-side, never leak it.
		fmt.Fprintf(os.Stderr, "hub: agent %q command %q failed: %v\n", name, args[0], err)
		if exit == 0 {
			exit = 1
		}
		// "Try again later" is only true of a fault that might pass. A broken project config never
		// will, and telling an agent otherwise bought hours of retries against a one-line fix: the
		// project is misconfigured, every verb that reads it is down, and only a human can end it.
		if errors.Is(err, config.ErrConfig) {
			return exit, fmt.Errorf("this project's .sindri/config.yaml can't be read, so %q and anything "+
				"else needing it will keep failing. Retrying won't help and there's nothing in /workspace "+
				"to fix — tell the user, and carry on with whatever doesn't need it", args[0])
		}
		return exit, fmt.Errorf("the hub hit an internal error running %q — it's logged for the operator; nothing for you to fix, try again later", args[0])
	}
	return exit, nil
}

func (h *Hub) cmdStatus(c registry.Caller, _ []string, out io.Writer) (int, error) {
	running := container.Running(h.container(c.Project, c.Agent))
	fmt.Fprintf(out, "agent:   %s\nrole:    %s\nrunning: %v\n", c.Agent, c.Role, running)
	return 0, nil
}

// cmdLint runs the quality gate host-side: with a pr-id, that PR's worktree (the reviewer's
// pre-verdict check); with none, the caller's own (the worker's pre-submit self-check).
func (h *Hub) cmdLint(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) > 0 { // lint a specific PR's worktree
		res, err := h.wf.LintPR(c.Project, args[0])
		if err != nil {
			return 1, err
		}
		fmt.Fprint(out, res)
		return 0, nil
	}
	ps := h.store.For(c.Project)
	a, ok, err := ps.GetAgent(c.Agent)
	if err != nil {
		return 1, err
	}
	if !ok {
		return 1, fmt.Errorf("unknown agent %q", c.Agent)
	}
	verify := ""
	if cfg, cerr := h.projectConfig(c.Project); cerr == nil {
		verify = cfg.Verify
	}
	res, passed := repo.Gate(filepath.Join(h.projectRoot(c.Project), a.Workspace), agent.BrokkrBinary, verify)
	if strings.TrimSpace(res) == "" {
		res = "lint: clean\n"
	}
	fmt.Fprint(out, res)
	if !passed {
		return 1, nil // non-zero so the agent knows the gate failed
	}
	return 0, nil
}

func (h *Hub) cmdLog(c registry.Caller, args []string, out io.Writer) (int, error) {
	msg := strings.TrimSpace(strings.Join(args, " "))
	if msg == "" {
		fmt.Fprintln(out, "usage: log <message>")
		return 2, nil
	}
	if err := h.store.For(c.Project).Log(c.Agent, "note", msg); err != nil {
		return 1, err
	}
	fmt.Fprintln(out, "logged")
	return 0, nil
}

// commentUsage is the argument form for this caller: no id where the caller's own state already names
// the task, both forms for a worker inside a feature, and the id required of a planner or coauthor,
// who address the whole backlog. A container is spelled out as the literal id to type.
func commentUsage(c registry.Caller) string {
	switch c.Role {
	case "worker":
		if c.Container != "" && c.Task != "" { // two in reach only once a subtask is actually assigned
			return fmt.Sprintf("comment <text...> (the subtask you're on), or comment %s <text...> (the feature)", c.Container)
		}
		return "comment <text...>"
	case "reviewer":
		return "comment <text...>"
	}
	return "comment <id> <text...>"
}

// approveHelp is the verb's help line, since what a verdict MEANS differs by role: a reviewer's
// opens the merge gate, a planner's is an optional badge beside it.
func approveHelp(c registry.Caller) string {
	if c.Role == "planner" {
		return "add an optional advisory approval badge, beside the reviewer's verdict, never instead of it: approve [pr-id]"
	}
	return "approve a pull request: approve [pr-id]"
}

// commentHelp is the verb's help line. A caller with no role yields the general form, which is what
// the registry carries as the static Help.
func commentHelp(c registry.Caller) string {
	switch c.Role {
	case "worker":
		return "comment on your current task: " + commentUsage(c)
	case "reviewer":
		return "comment on the task of the PR you're reviewing: " + commentUsage(c)
	}
	return "comment on a task: " + commentUsage(c)
}

// commentTarget is the task a caller's own state already names, "" for a role where nothing does. A
// worker inside a feature has two, and the subtask wins — that is what it has open when it finds
// something. The container stays reachable by its id, and the reply says which one was written to.
func (h *Hub) commentTarget(c registry.Caller) (string, error) {
	switch c.Role {
	case "worker":
		if c.Task != "" {
			return c.Task, nil
		}
		return c.Container, nil
	case "reviewer":
		ps := h.store.For(c.Project)
		pr, err := ps.ReviewingPR(c.Agent)
		if err != nil {
			return "", err
		}
		p, ok, err := ps.GetPR(pr)
		if err != nil {
			return "", err
		}
		if !ok {
			// A missing row for an assigned PR is a hub fault, not something the agent can act on.
			return "", fmt.Errorf("agent %q is reviewing PR %q, which is not in the store", c.Agent, pr)
		}
		return p.Task, nil
	}
	return "", nil // planner, coauthor: the whole backlog is in reach, so no single task is implied
}

// cmdComment posts a comment through the same service the front-ends' `task comment` writes to. The
// id is OPTIONAL wherever the caller's state names a task (-> commentTarget); a first argument shaped
// like an id is the explicit form, anything else starts the text. Scope is checked against what
// caller() resolved, never trusted from the argument.
func (h *Hub) cmdComment(c registry.Caller, args []string, out io.Writer) (int, error) {
	target, err := h.commentTarget(c)
	if err != nil {
		return 1, err
	}
	id, body := target, strings.Join(args, " ")
	if len(args) > 0 && task.IsID(args[0]) {
		id, body = args[0], strings.Join(args[1:], " ")
	}
	if id == "" || strings.TrimSpace(body) == "" {
		fmt.Fprintf(out, "usage: %s\n", commentUsage(c))
		return 2, nil
	}
	ps := h.store.For(c.Project)
	switch c.Role {
	case "worker":
		if id != c.Task && id != c.Container {
			fmt.Fprintf(out, "%s isn't the task you hold — you can only comment on that\n", id)
			return 1, nil
		}
	case "reviewer":
		if id != target {
			fmt.Fprintf(out, "%s isn't the task of the PR you're reviewing — you can only comment on that\n", id)
			return 1, nil
		}
	default: // planner, coauthor: any task in this project — they already read the whole backlog
		if _, ok, err := ps.GetTask(id); err != nil {
			return 1, err
		} else if !ok {
			fmt.Fprintf(out, "no such task %q\n", id)
			return 1, nil
		}
	}
	if err := h.comments.Add(c.Project, id, c.Agent, body); err != nil {
		fmt.Fprintf(out, "%v\n", err) // an empty body, say, is the agent's to fix, not a hub fault
		return 1, nil
	}
	// With the id optional the agent may not have typed the target, and inside a feature two are in
	// reach — so name it, and say which of the two.
	which := ""
	if c.Container != "" {
		switch id {
		case c.Task:
			which = " (the subtask you're on)"
		case c.Container:
			which = " (the feature you hold)"
		}
	}
	fmt.Fprintf(out, "commented on %s%s\n", id, which)
	return 0, nil
}

func (h *Hub) cmdListPRs(c registry.Caller, _ []string, out io.Writer) (int, error) {
	ps := h.store.For(c.Project)
	prs, err := ps.PRs()
	if err != nil {
		return 1, err
	}
	if len(prs) == 0 {
		fmt.Fprintln(out, "no PRs")
		return 0, nil
	}
	counts, err := ps.ApprovalCounts()
	if err != nil {
		return 1, err
	}
	for _, p := range prs {
		fmt.Fprintf(out, "%-14s %-14s %-10s %s\n", p.ID, api.StatusLabel(p.Status, counts[p.ID]), p.Agent, p.Branch)
	}
	return 0, nil
}

// cmdChat is the agent-facing `chat` verb — it delegates to the chat relay.
func (h *Hub) cmdChat(c registry.Caller, args []string, out io.Writer) (int, error) {
	return h.chat.Cmd(c, args, out)
}

// reopenTaskUsage is the one description of reopen-task's surface, shown for a missing reason and
// (via reopenTaskHelp) `reopen-task --help`.
const reopenTaskUsage = "usage: reopen-task <id> <reason...>\n" +
	"  Restores a closed task sindri owns (sd-/td-) to open. The reason is required — it is\n" +
	"  recorded as a comment on the task, and is the whole point: it is what tells the next\n" +
	"  reader this was a failed verification rather than a change of mind.\n" +
	"  Refused for a task whose status comes from its own source (an openspec change os-*, a\n" +
	"  GitHub issue gh-*) — reopen those there.\n" +
	"  No new approval is created: reopening restores the release the task already had, so a\n" +
	"  task that still carries a priority becomes claimable again immediately."

// reopenTaskHelp is what the command registry advertises for reopen-task.
const reopenTaskHelp = "reopen a closed task, with a reason. " + reopenTaskUsage

// cmdReopenTask is the planner-facing `reopen-task <id> <reason...>` verb.
func (h *Hub) cmdReopenTask(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) < 2 {
		fmt.Fprintln(out, reopenTaskUsage)
		return 2, nil
	}
	id, reason := args[0], strings.Join(args[1:], " ")
	if err := h.ReopenTask(c.Project, id, c.Agent, reason); err != nil {
		fmt.Fprintf(out, "could not reopen %s: %v\n", id, err)
		return 1, nil
	}
	fmt.Fprintf(out, "%s reopened.\n", id)
	// The one thing the caller must be told rather than discover: a priority left standing from
	// before the close is enough on its own to make this immediately claimable by a worker.
	if t, ok, terr := h.store.For(c.Project).GetTask(id); terr == nil && ok && t.Priority != "" {
		fmt.Fprintln(out, "It still carries a priority, so a worker may claim it immediately.")
	}
	return 0, nil
}

// ReopenTask restores a closed sindri-owned task (-> workflow.Engine.ReopenTask) and records reason
// as a comment on it, attributed to author — the "this did not hold" signal a fresh duplicate task
// would otherwise lose. Both cmdReopenTask and server.go's /task/reopen route through here, so the
// reason is required, and recorded exactly once, however it was asked for.
func (h *Hub) ReopenTask(project, id, author, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("say why %s is being reopened — an unstated reason is exactly what gets lost otherwise", id)
	}
	if err := h.wf.ReopenTask(project, id); err != nil {
		return err
	}
	// The task is already open by this point: say so, or the error alone reads as "neither happened".
	if err := h.comments.Add(project, id, author, "reopened: "+reason); err != nil {
		return fmt.Errorf("%s was reopened, but recording the reason failed: %w", id, err)
	}
	return nil
}
