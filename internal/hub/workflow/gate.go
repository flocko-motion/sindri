// package: hub/workflow / gate
// type:    logic (the submit/contribute quality gate, routed through the run queue)
// job:     run a submit/contribute gate from the queue instead of inline, and land the
// continuation its result unlocks — a PR on pass, feedback to fix on fail, a plain
// "try again" on a gate that never reached a verdict (timeout, or a hub restart).
// limits:  the gate check itself and what follows it. Queue mechanics (ranking,
// dequeue, one-at-a-time) live in run.go/execrun.go/hub/runwatch.go.
package workflow

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/repo"
	"github.com/flo-at/sindri/internal/hub/store"
)

// enqueueGate queues a submit/contribute gate, snapshotting the agent's free-text description
// (r.Message) so the eventual commit reads the same as an inline gate's would have.
func (e *Engine) enqueueGate(project, agent, kind, message string) (api.Run, error) {
	return e.putQueuedRun(project, agent, kind, "gate: "+kind, message, "", "")
}

// executeGateRun runs a submit/contribute gate from the queue: repo.Gate directly against the
// agent's live worktree, no container and no materialization — it is checking exactly what the
// agent currently has, the same worktree an inline gate always checked. Its own 15-minute verify
// timeout (repo.GateTimeout, already equal to RunHardCap) is the bound; nothing here adds a second.
func (e *Engine) executeGateRun(ps *store.ProjectStore, project string, r api.Run) error {
	root := e.deps.ProjectRoot(project)
	a, _, _ := ps.GetAgent(r.Agent) // existence already checked by staleReason
	wt := filepath.Join(root, a.Workspace)
	if err := ps.SetRunStatus(r.ID, "running"); err != nil {
		return err
	}
	e.deps.Notify()
	start := time.Now()
	out, passed := repo.Gate(wt, e.deps.BrokkrBin, e.verifyCmd(project))
	status, exitCode := "passed", 0
	if !passed {
		status, exitCode = "failed", 1
	}
	return e.finishRun(ps, project, r, status, out, time.Since(start), RunHardCap, exitCode)
}

// completeGate is the continuation a gate result unlocks. Only "passed" creates a PR — the
// guarantee a failing PR is never created holds exactly the way it did when the gate ran inline.
func (e *Engine) completeGate(project string, r api.Run, status, output string) error {
	ps := e.store.For(project)
	switch status {
	case "passed":
		return e.landGate(project, ps, r)
	case "failed":
		return e.rejectGate(project, ps, r, output)
	default: // timed_out, cancelled: the gate never reached a verdict — not a finding about the code
		return e.stallGate(project, ps, r, status)
	}
}

// landGate dispatches to the kind-specific continuation — the exact tail CmdSubmit/CmdContribute
// ran right after an inline gate passed, now re-derived from current state instead of a live call.
func (e *Engine) landGate(project string, ps *store.ProjectStore, r api.Run) error {
	switch r.Kind {
	case "contribute":
		_, err := e.openMilestoneOrInterim(project, r)
		return err
	default: // "submit"
		return e.landSubmit(project, ps, r)
	}
}

// openMilestoneOrInterim lands a passed contribute gate: the feature branch as a milestone if the
// agent holds one, otherwise this task's interim contribution — the same fork CmdContribute made
// before the gate became a queued step.
func (e *Engine) openMilestoneOrInterim(project string, r api.Run) (store.PR, error) {
	ps := e.store.For(project)
	st, err := ps.GetState(r.Agent)
	if err != nil {
		return store.PR{}, err
	}
	if st.Container != "" {
		pr, err := e.openMilestone(project, r.Agent, r.Message)
		if err != nil {
			return store.PR{}, err
		}
		_ = e.deps.InjectWhenReady(project, r.Agent, "[hub] "+ReplyMilestoneContributed(pr.ID, st.Container))
		return pr, nil
	}
	root := e.deps.ProjectRoot(project)
	a, _, _ := ps.GetAgent(r.Agent)
	wt := filepath.Join(root, a.Workspace)
	tk, _, _ := ps.GetTask(st.Task)
	msg := r.Message
	if msg == "" {
		msg = tk.Title
	}
	if msg == "" {
		msg = "interim contribution on " + st.Task
	}
	msg = conventionalCommit(tk.Type, st.Task, msg)
	if err := git.CommitAll(wt, msg); err != nil {
		return store.PR{}, err
	}
	base, err := e.baseBranch(root)
	if err != nil {
		return store.PR{}, err
	}
	pr := store.PR{ID: "pr-" + st.Task, Task: st.Task, Agent: r.Agent, Branch: st.Branch, Base: base, Status: "open", Kind: "interim"}
	_, existed, _ := ps.GetPR(pr.ID)
	if err := ps.PutPR(pr); err != nil {
		return store.PR{}, err
	}
	// A conflict here would need the resolve loop, exactly as it does inline — but the gate already
	// ran against the tree this rebase now replays, so the rare conflict left here still lands the
	// agent in "resolving" the same way, just one step later than an inline gate would have.
	conflicts, done, err := repo.RebaseStep(wt, st.Branch, base)
	if err != nil {
		return store.PR{}, err
	}
	if !done {
		_ = ps.SetState(store.AgentState{Agent: r.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "resolving"})
		_ = ps.Log(r.Agent, "contribute-conflict", strings.Join(conflicts, ", "))
		_ = e.deps.InjectWhenReady(project, r.Agent, "[hub] "+ReplyContributeConflicts(base, conflicts))
		return pr, nil
	}
	if err := ps.SetState(store.AgentState{Agent: r.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "submitted"}); err != nil {
		return store.PR{}, err
	}
	_ = ps.Log(r.Agent, "contribute", pr.ID)
	if existed {
		_ = ps.LogPR(pr.ID, "resubmitted", "interim, by "+r.Agent+": "+msg)
	} else {
		_ = ps.LogPR(pr.ID, "created", "interim, by "+r.Agent+": "+msg)
	}
	e.deps.Notify()
	_ = e.deps.InjectWhenReady(project, r.Agent, "[hub] "+ReplyContributed(pr.ID))
	return pr, nil
}

// landSubmit lands a passed submit gate: commit, record the PR, request review, tell the agent.
func (e *Engine) landSubmit(project string, ps *store.ProjectStore, r api.Run) error {
	st, err := ps.GetState(r.Agent)
	if err != nil {
		return err
	}
	target, branch := st.Task, st.Branch
	if st.Container != "" {
		target, branch = st.Container, st.Container
	}
	root := e.deps.ProjectRoot(project)
	a, _, _ := ps.GetAgent(r.Agent)
	wt := filepath.Join(root, a.Workspace)
	base, err := e.baseBranch(root)
	if err != nil {
		return err
	}
	tk, _, _ := ps.GetTask(target)
	desc := r.Message
	if desc == "" {
		desc = tk.Title
	}
	if desc == "" {
		desc = "work on " + target
	}
	msg := conventionalCommit(tk.Type, target, desc)
	if err := git.CommitAll(wt, msg); err != nil {
		return err
	}
	pr := store.PR{ID: "pr-" + target, Task: target, Agent: r.Agent, Branch: branch, Base: base, Status: "open"}
	_, existed, _ := ps.GetPR(pr.ID)
	if err := ps.PutPR(pr); err != nil {
		return err
	}
	if err := ps.SetState(store.AgentState{
		Agent: r.Agent, Task: st.Task, Branch: branch, Container: st.Container, Phase: "submitted",
	}); err != nil {
		return err
	}
	_ = ps.Log(r.Agent, "submit", pr.ID)
	if existed {
		_ = ps.LogPR(pr.ID, "resubmitted", "by "+r.Agent+": "+msg)
	} else {
		_ = ps.LogPR(pr.ID, "created", "by "+r.Agent+": "+msg)
	}
	if err := e.RequestReview(project, pr.ID, ""); err != nil {
		_ = ps.Log(r.Agent, "review-request-failed", pr.ID+": "+err.Error())
		return e.deps.InjectWhenReady(project, r.Agent, "[hub] "+ReplyReviewRequestFailed(pr.ID, err))
	}
	return e.deps.InjectWhenReady(project, r.Agent, MsgGatePassed(pr.ID))
}

// rejectGate lands a failed gate: back to "working" with the violations, exactly what an inline
// refusal left the agent to fix — only the delivery (injected, not a command reply) differs.
func (e *Engine) rejectGate(project string, ps *store.ProjectStore, r api.Run, output string) error {
	st, _ := ps.GetState(r.Agent)
	if err := ps.SetState(store.AgentState{Agent: r.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "working"}); err != nil {
		return err
	}
	_ = ps.Log(r.Agent, "lint-fail", gateTarget(st))
	e.deps.Notify()
	return e.deps.InjectWhenReady(project, r.Agent, MsgGateFailed(strings.TrimSpace(output)))
}

// stallGate lands a gate that never reached a verdict: back to "working", told to just try again
// — never as a lint failure, or a flaky timeout reads as a violation and an agent "fixes" nothing.
func (e *Engine) stallGate(project string, ps *store.ProjectStore, r api.Run, status string) error {
	st, _ := ps.GetState(r.Agent)
	if err := ps.SetState(store.AgentState{Agent: r.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "working"}); err != nil {
		return err
	}
	_ = ps.Log(r.Agent, "gate-incomplete", status)
	e.deps.Notify()
	return e.deps.InjectWhenReady(project, r.Agent, MsgGateIncomplete(status))
}

// gateTarget names what a gate was checking, for the activity log — the container if the agent
// holds one, the task otherwise.
func gateTarget(st store.AgentState) string {
	if st.Container != "" {
		return st.Container
	}
	return st.Task
}
