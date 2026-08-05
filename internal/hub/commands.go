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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/repo"
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
			Blocked: func(c registry.Caller) string {
				if c.HasTask {
					return "You already hold work — run `sindri` to be told what to do with it."
				}
				return ""
			}, Run: h.wf.CmdNext},
		registry.Command{Name: "lint", Help: "run the quality gate: lint (your workspace) or lint <pr-id> (a PR)", Run: h.cmdLint},
		// Visibility MUST match these commands' own `st.Phase != "working"` guard. When it didn't, a
		// worker in "submitted" was offered submit, ran it, and was told to abandon the task it held.
		registry.Command{Name: "submit", Help: "request your branch be merged: submit [message]", Roles: []string{"worker"},
			Blocked: landingBlocked("submit"), Run: h.wf.CmdSubmit},
		// Land interim work mid-task without finishing it; same visibility as submit, task stays open.
		registry.Command{Name: "contribute", Help: "land an interim contribution mid-task (needs the user's approval): contribute [message]", Roles: []string{"worker"},
			Blocked: landingBlocked("contribute"), Run: h.wf.CmdContribute},
		// Always available to a worker — checking your branch still merges is harmless at any time.
		registry.Command{Name: "resolve", Help: "check your branch still merges onto its base, and resolve any conflicts: resolve", Roles: []string{"worker"}, Run: h.wf.CmdResolve},
		// Align any time — harmless, and it surfaces conflicts to fix rather than letting drift.
		registry.Command{Name: "rebase", Help: "rebase your branch onto the current reference branch (fix any conflicts it surfaces): rebase", Roles: []string{"worker", "planner"}, Run: h.wf.CmdRebase},
		// The agents have no git of their own — that isolation is the point. So the hub runs a
		// curated read/restore subset for them (-> workflow.CmdGit): without it, an agent cannot
		// see what it changed or put a file back, and reconstructs both from memory.
		registry.Command{Name: "git", Help: workflow.GitHelp, Roles: []string{"worker", "planner", "coauthor"}, Run: h.wf.CmdGit},
		registry.Command{Name: "checkpoint", Help: "record the current subtask and move to the next: checkpoint [summary]", Roles: []string{"worker"},
			Blocked: func(c registry.Caller) string {
				if c.Container == "" {
					return "Checkpoint records one subtask of a feature, and you hold a task of your own — " +
						"`sindri submit \"<summary>\"` puts it up for review when it's done."
				}
				return ""
			}, Run: h.wf.CmdCheckpoint},
		// A worker reads too: it holds a whole package for context, so that context must stay
		// re-readable. Roles see different scopes (-> CmdTasks) but share one verb name.
		registry.Command{Name: "task", Help: workflow.TaskHelp, Roles: []string{"planner", "coauthor", "worker"}, Run: h.wf.CmdTasks},
		registry.Command{Name: "create-task", Help: workflow.CreateTaskHelp, Roles: []string{"planner"}, Run: h.wf.CmdCreateTask},
		registry.Command{Name: "edit-task", Help: workflow.EditTaskHelp, Roles: []string{"planner"}, Run: h.wf.CmdEditTask},
		registry.Command{Name: "openspec", Help: "ship your openspec changes as a PR: openspec submit [message]", Roles: []string{"planner"}, Run: h.wf.CmdOpenspec},
		registry.Command{Name: "state", Help: "set your resting state: state planning | state idle", Roles: []string{"planner"}, Run: h.wf.CmdState},
		registry.Command{Name: "approve", Help: "approve a pull request: approve [pr-id]", Roles: []string{"reviewer"}, Run: h.wf.CmdApprove},
		registry.Command{Name: "reject", Help: "reject a pull request: reject <pr-id> <feedback...>", Roles: []string{"reviewer"}, Run: h.wf.CmdReject},
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
// Inside a feature the unit that goes up is the whole branch, so submit waits for the last subtask
// and contribute has no role at all — checkpoint is the interim landing there. Outside one, there is
// nothing to land except from "working", worded exactly as the verb's own guard words it so an agent
// hears one story whichever gate it meets first.
func landingBlocked(verb string) func(registry.Caller) string {
	return func(c registry.Caller) string {
		if c.Container != "" {
			switch {
			case verb == "contribute":
				return fmt.Sprintf("Inside feature %s, `sindri checkpoint \"<summary>\"` is how you land "+
					"work as you go — it records the subtask on the feature branch without ending anything.",
					c.Container)
			case c.SubtasksOpen:
				return fmt.Sprintf("Feature %s still has open subtasks, and it goes up as ONE PR — "+
					"record the one you're on with `sindri checkpoint \"<summary>\"` and it will hand you "+
					"the next. Submit once they're all done.", c.Container)
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
		open, oerr := ps.OpenChildren(st.Container)
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

// AgentCommands returns the command surface currently available to an agent.
func (h *Hub) AgentCommands(project, name string) ([]CmdInfo, error) {
	c, err := h.caller(project, name)
	if err != nil {
		return nil, err
	}
	avail := h.registry().Available(c)
	out := make([]CmdInfo, len(avail))
	for i, cmd := range avail {
		out[i] = CmdInfo{Name: cmd.Name, Help: cmd.Help}
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
		fmt.Fprintf(out, "%s\n", cmd.Help)
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
	res, passed := repo.Lint(filepath.Join(h.projectRoot(c.Project), a.Workspace), agent.BrokkrBinary)
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

func (h *Hub) cmdListPRs(c registry.Caller, _ []string, out io.Writer) (int, error) {
	prs, err := h.store.For(c.Project).PRs()
	if err != nil {
		return 1, err
	}
	if len(prs) == 0 {
		fmt.Fprintln(out, "no PRs")
		return 0, nil
	}
	for _, p := range prs {
		fmt.Fprintf(out, "%-14s %-9s %-10s %s\n", p.ID, p.Status, p.Agent, p.Branch)
	}
	return 0, nil
}

// cmdChat is the agent-facing `chat` verb — it delegates to the chat relay.
func (h *Hub) cmdChat(c registry.Caller, args []string, out io.Writer) (int, error) {
	return h.chat.Cmd(c, args, out)
}
