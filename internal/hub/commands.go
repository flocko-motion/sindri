// package: hub / commands
// type:    logic (the hub-side verb set the browser invokes)
// job:     build the command registry with hub-bound behaviour, resolve a
//
//	caller's identity/role/state, and execute a verb on its behalf
//	(logging the socket call to the activity log). Phase 2 ships the
//	mechanism with real `status`/`log` plus role-scoped Phase-3 stubs.
//
// limits:  workflow verbs (submit/next/approve/reject) gain real behaviour in
//
//	Phase 3; here they exist only to prove surface filtering.
package hub

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/pod"
	"github.com/flo-at/sindri/internal/hub/registry"
)

// CmdInfo is a command as advertised to a browser (name + help).
type CmdInfo struct {
	Name string `json:"name"`
	Help string `json:"help"`
}

// registry builds the command surface. Rebuilt per call (cheap); Run closures
// capture the hub so commands can reach the store/adapters.
func (h *Hub) registry() *registry.Registry {
	return registry.New(
		registry.Command{Name: "status", Help: "show who you are and your current state", Run: h.cmdStatus},
		registry.Command{Name: "log", Help: "record a note in your activity log: log <message>", Run: h.cmdLog},
		registry.Command{Name: "prs", Help: "list pull requests and their status", Run: h.cmdListPRs},
		registry.Command{Name: "show", Help: "show a PR's diff: show <pr-id>", Run: h.cmdShowPR},
		registry.Command{Name: "next", Help: "pick up the next task", Roles: []string{"worker", "planner"},
			Hidden: func(c registry.Caller) bool { return c.HasTask }, Run: h.cmdNext},
		registry.Command{Name: "lint", Help: "run the quality gate: lint (your workspace) or lint <pr-id> (a PR)", Run: h.cmdLint},
		registry.Command{Name: "submit", Help: "request your branch be merged: submit [message]", Roles: []string{"worker", "planner"},
			Hidden: func(c registry.Caller) bool { return !c.HasTask }, Run: h.cmdSubmit},
		registry.Command{Name: "tasks", Help: "read the backlog: tasks (list all) or tasks <id> (full detail)", Roles: []string{"planner"}, Run: h.cmdTasks},
		registry.Command{Name: "create-task", Help: "propose a new task (needs the user's approval): create-task <title...>", Roles: []string{"planner"}, Run: h.cmdCreateTask},
		registry.Command{Name: "approve", Help: "approve a pull request: approve [pr-id]", Roles: []string{"reviewer"}, Run: h.cmdApprove},
		registry.Command{Name: "reject", Help: "reject a pull request: reject <pr-id> <feedback...>", Roles: []string{"reviewer"}, Run: h.cmdReject},
		registry.Command{Name: "review", Help: "record a review verdict: review <pr-id> <pass|changes|fail> <findings...>", Roles: []string{"reviewer"}, Run: h.cmdReview},
	)
}

// caller resolves an agent's identity and role (workflow state arrives Phase 3).
func (h *Hub) caller(name string) (registry.Caller, error) {
	a, ok, err := h.store.GetAgent(name)
	if err != nil {
		return registry.Caller{}, err
	}
	if !ok {
		return registry.Caller{}, fmt.Errorf("unknown agent %q", name)
	}
	// A worker holding a task (working or submitted) hides "next" and shows
	// "submit"; an idle worker the reverse (state machine, D-hub).
	st, err := h.store.GetState(name)
	if err != nil {
		return registry.Caller{}, err
	}
	return registry.Caller{Agent: name, Role: a.Role, HasTask: st.Phase != "idle"}, nil
}

// AgentCommands returns the command surface currently available to an agent.
func (h *Hub) AgentCommands(name string) ([]CmdInfo, error) {
	c, err := h.caller(name)
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

// AgentExec runs a verb on behalf of an agent, streaming to out and returning a
// process-style exit code. Every call is recorded in the activity log (the
// socket "messages sent", D12).
func (h *Hub) AgentExec(name string, args []string, out io.Writer) (int, error) {
	c, err := h.caller(name)
	if err != nil {
		return 1, err
	}
	if len(args) == 0 {
		return 1, fmt.Errorf("no command given")
	}
	// Note: command invocations aren't logged as activity — the meaningful ones
	// record their own outcome (claim/submit/note/approve/reject/merged); reads
	// (status/prs/show) are not activity at all.
	cmd, ok := h.registry().Lookup(args[0], c)
	if !ok {
		fmt.Fprintf(out, "unknown or unavailable command: %s\n", args[0])
		return 127, nil
	}
	exit, err := cmd.Run(c, args[1:], out)
	h.notify() // the command may have changed board state
	return exit, err
}

func (h *Hub) cmdStatus(c registry.Caller, _ []string, out io.Writer) (int, error) {
	running := pod.Running(h.container(c.Agent))
	fmt.Fprintf(out, "agent:   %s\nrole:    %s\nrunning: %v\n", c.Agent, c.Role, running)
	return 0, nil
}

// cmdLint runs the quality gate (host-side) and streams the result. With a
// pr-id it lints that PR's worktree (the reviewer's pre-verdict check); with no
// args it lints the caller's own worktree (the worker's pre-submit self-check).
func (h *Hub) cmdLint(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) > 0 { // lint a specific PR's worktree
		res, err := h.LintPR(args[0])
		if err != nil {
			return 1, err
		}
		fmt.Fprint(out, res)
		return 0, nil
	}
	a, ok, err := h.store.GetAgent(c.Agent)
	if err != nil {
		return 1, err
	}
	if !ok {
		return 1, fmt.Errorf("unknown agent %q", c.Agent)
	}
	res, passed := h.runLint(filepath.Join(h.root, a.Workspace))
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
	if err := h.store.Log(c.Agent, "note", msg); err != nil {
		return 1, err
	}
	fmt.Fprintln(out, "logged")
	return 0, nil
}

func (h *Hub) cmdListPRs(_ registry.Caller, _ []string, out io.Writer) (int, error) {
	prs, err := h.store.PRs()
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
