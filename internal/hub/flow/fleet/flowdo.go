// package: hub/flow/fleet / flowdo
// type:    logic (the implementations behind the declared actions)
// job:     one function per action a state can run, registered against the identity the flow file
// names. This is the only place in the flow that writes anything or touches the harness.
// limits:  doing. WHICH action runs where is the flow's declaration (-> hub/flow/roles).
package fleet

import (
	"context"
	"fmt"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"log"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// doers is the implementation of every declared action, by name. A declared action with no entry
// here fails at startup rather than at an agent's next ask (-> machine.New's check).
func (e *Engine) doers() map[string]machine.Doer[flow.World] {
	return map[string]machine.Doer[flow.World]{
		act.PickWork.Name:    e.doPickWork,
		act.PickSubtask.Name: e.doPickSubtask,
		act.Clear.Name:       e.doClear,
		act.Retier.Name:      e.doRetier,
		act.Yield.Name:       e.doYield,
		act.Release.Name:     e.doRelease,
		act.Submit.Name:      e.doSubmit,
		act.TakeReview.Name:  e.doTakeReview,
		act.DropReview.Name:  e.doDropReview,
		act.Prod.Name:        e.doProd,
		act.Promote.Name:     e.doPromote,
		act.Rebase.Name:      e.doRebase,
	}
}

// doPickWork claims the best-rated unit the backlog would hand this agent and delivers its brief.
// The claim comes FIRST, so holding the work protects it while the preparation behind it runs — and
// so a pod that is down or mid-restart still HOLDS the work it was given: delivery reports whether
// the brief landed, and the agent is told what it holds on its next ask either way.
func (e *Engine) doPickWork(ctx context.Context, w flow.World) (flow.Outcome, error) {
	if !w.HasNext {
		return act.Nothing, nil
	}
	var dir string
	var err error
	if w.NextIsFeature {
		dir, _, err = e.taskAct().ClaimContainer(w.Project, w.Name, w.Next)
	} else {
		dir, _, err = e.taskAct().ClaimLeaf(w.Project, w.Name, w.Next)
	}
	if err != nil {
		return act.Held, err
	}
	// The preparation runs INSIDE the hand-over: a model switch, else a clear, and the brief goes out
	// behind whichever landed. One state covers the whole of it, so an event arriving mid-way cancels
	// the hand-over rather than catching it between two states.
	fired, perr := e.roleAct().PrepareAssignment(ctx, w.Project, w.Name, api.TierOrDefault(w.Next.Tier), dir)
	if perr != nil {
		return act.Held, perr
	}
	if fired {
		return act.Done, nil // delivered behind the preparation
	}
	return act.Done, e.Harness.Say(w.Project, w.Name, dir, mail.PushOnly)
}

// doPickSubtask moves a feature holder onto its feature's next open child.
func (e *Engine) doPickSubtask(ctx context.Context, w flow.World) (flow.Outcome, error) {
	if len(w.Subtasks) == 0 {
		return act.Nothing, nil
	}
	child := w.Subtasks[0]
	if err := e.taskAct().StartSubtask(w.Project, w.Name, w.Container, child); err != nil {
		return act.Nothing, err
	}
	dir := prompts.DirContainerWorking(w.Container, child.ID, w.Aim, w.Ceiling)
	if fired, perr := e.roleAct().PrepareAssignment(ctx, w.Project, w.Name, api.TierOrDefault(child.Tier), dir); perr != nil || fired {
		return act.Done, perr
	}
	return act.Done, e.Harness.Say(w.Project, w.Name, dir, mail.PushOnly)
}

// doClear discards the session and waits for the reading to fall — the clear having HAPPENED, where
// a sleep only assumes it. A clear that never lands must not withhold the work.
func (e *Engine) doClear(ctx context.Context, w flow.World) (flow.Outcome, error) {
	// The request is answered the moment this runs, whatever the clear then does. Left armed, the
	// condition that brought the agent here holds again the instant it leaves.
	e.disarmClear(w.Project, w.Name)
	if w.Fill == 0 {
		return act.Done, nil // nothing recorded: a session nobody has read yet has nothing to drop
	}
	if err := e.Harness.Clear(ctx, w.Project, w.Name); err != nil {
		_ = e.Store.For(w.Project).Log(w.Name, "clear-unconfirmed", err.Error())
		return act.Failed, nil
	}
	// Nothing is said into the empty session: this outcome moves the agent on at once, and where it
	// lands either acts (a hand-over speaks its own brief) or tells the agent itself (-> tell).
	return act.Done, nil
}

// doRetier switches the model under the agent for the tier of the work coming. The switch clears the
// session on its way through, so nothing said into it survives.
func (e *Engine) doRetier(ctx context.Context, w flow.World) (flow.Outcome, error) {
	want, known := e.Deps.ModelForTier(api.TierOrDefault(w.Next.Tier))
	if !known || e.Harness.ModelMatches(want, w.Model) {
		return act.Done, nil
	}
	if err := e.Harness.SetModel(ctx, w.Project, w.Name, want); err != nil {
		_ = e.Store.For(w.Project).Log(w.Name, "retier-failed", err.Error())
		return act.Failed, nil
	}
	return act.Done, nil
}

// doYield frees a container holder whose tree somebody else is inside. The CONTAINER holder gives
// way: the leaf is the concrete work.
func (e *Engine) doYield(_ context.Context, w flow.World) (flow.Outcome, error) {
	e.taskAct().HealSplit(w.Project, w.Name)
	return act.Done, nil
}

// doRelease drops a feature that has already landed and returns the agent to the backlog.
func (e *Engine) doRelease(_ context.Context, w flow.World) (flow.Outcome, error) {
	ps := e.Store.For(w.Project)
	err := ps.SetState(store.AgentState{Agent: w.Name, Phase: w.Phase},
		store.ReasonLanded, "feature already landed: "+w.Container)
	return act.Done, err
}

// doSubmit takes what the agent has: the quality gate first, then a pull request.
func (e *Engine) doSubmit(ctx context.Context, w flow.World) (flow.Outcome, error) {
	return act.Queued, fmt.Errorf("submit is still routed through the verb surface")
}

// doProd wakes an agent that holds work and has stopped doing it. Not relieved of the work: a stall
// is an agent that needs waking, not one that has failed.
func (e *Engine) doProd(_ context.Context, w flow.World) (flow.Outcome, error) {
	if !e.roleAct().NudgeStalled(w.Project, w.Name, w.Observation, w.StillFor) {
		return act.Nothing, nil // already prodded for this spell
	}
	return act.Done, nil
}

// doPromote turns the leaf an agent holds into the feature it has become.
func (e *Engine) doPromote(_ context.Context, w flow.World) (flow.Outcome, error) {
	e.taskAct().PromoteToFeature(w.Project, w.Name, w.Task)
	if st, err := e.Store.For(w.Project).GetState(w.Name); err != nil || st.Container == "" {
		return act.Failed, err
	}
	return act.Done, nil
}

// doRebase catches a standing branch up with the base its own milestone merge moved, keeping
// whatever is mid-edit. A reset rather than a rebase: the merge was squashed, so the old commits are
// unrecognisable to one (-> git.ResetOntoKeepingWork).
func (e *Engine) doRebase(_ context.Context, w flow.World) (flow.Outcome, error) {
	ps := e.Store.For(w.Project)
	a, ok, err := ps.GetAgent(w.Name)
	if err != nil || !ok {
		return act.Failed, err
	}
	base, berr := e.BaseBranch(e.Deps.ProjectRoot(w.Project))
	if berr != nil {
		return act.Failed, berr
	}
	wt := filepath.Join(e.Deps.ProjectRoot(w.Project), a.Workspace)
	// Recorded FIRST, whatever happens next: an attempt that failed still happened, and a condition
	// that stays true because the attempt did not succeed resets the same branch on every beat.
	if w.AwaitingPR != "" {
		_ = ps.LogPR(w.AwaitingPR, "resumed", "author caught up with "+base)
	} else if last := e.lastMilestone(ps, w); last != "" {
		_ = ps.LogPR(last, "resumed", "author caught up with "+base)
	}
	conflicts, done, rerr := git.ResetOntoKeepingWork(wt, base)
	if rerr != nil {
		_ = ps.Log(w.Name, "reset-failed", "onto "+base+": "+rerr.Error())
		_ = e.Harness.Say(w.Project, w.Name, prompts.MsgResetFailed(w.AwaitingPR, base), mail.MailAndPush)
		return act.Failed, nil
	}
	if !done {
		_ = ps.Log(w.Name, "resolve", "reapplying uncommitted work onto "+base+" conflicts: "+strings.Join(conflicts, ", "))
		_ = e.Harness.Say(w.Project, w.Name, prompts.MsgReapplyConflict(w.AwaitingPR, base, conflicts), mail.MailAndPush)
		return act.Failed, nil
	}
	return act.Done, nil
}

// doTakeReview claims the oldest unclaimed review and puts its branch in the reviewer's workspace.
func (e *Engine) doTakeReview(ctx context.Context, w flow.World) (flow.Outcome, error) {
	var id int64
	var prID string
	// The project the ROW is in, which for a pooled reviewer is never its own (-> store.UnclaimedReview).
	home, found, err := e.Store.UnclaimedReview(w.Project, &id, &prID)
	if err != nil {
		return act.Failed, err
	}
	if !found {
		return act.Nothing, nil
	}
	req, _ := e.prAct().ReviewPrompt(home)
	if err := e.prAct().AssignReview(ctx, home, id, prID, w.Name, req); err != nil {
		return act.Failed, err
	}
	return act.Done, nil
}

// doDropReview releases a review whose PR settled before a verdict: one given now decides nothing.
func (e *Engine) doDropReview(_ context.Context, w flow.World) (flow.Outcome, error) {
	held := w.ReviewingPR
	heldProject, _, err := e.Store.ReviewingPR(w.Project, w.Name)
	if err != nil {
		return act.Done, err
	}
	hps := e.Store.For(heldProject)
	if err := hps.CloseReviews(held, "overtaken: the PR settled before a verdict"); err != nil {
		return act.Done, err
	}
	_ = e.Harness.Say(w.Project, w.Name, prompts.MsgReviewCancelled(held), mail.MailAndPush)
	return act.Done, nil
}

// disarmClear takes back the arming a clear has just answered. Written here rather than left to the
// harness: the harness disarms when a clear LANDS, and this state is reached — and left — on paths
// where it never does, which is how the same request kept bringing the agent back.
func (e *Engine) disarmClear(project, name string) {
	ps := e.Store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil || !ok || !a.ClearArmed {
		return
	}
	a.ClearArmed = false
	if perr := ps.PutAgent(a); perr != nil {
		log.Printf("hub: disarming %s's clear: %v", name, perr)
	}
}

// lastMilestone is the merged interim pull request this agent has not caught up with, "" if none.
func (e *Engine) lastMilestone(ps *store.ProjectStore, w flow.World) string {
	prs, err := ps.PRs("merged")
	if err != nil {
		return ""
	}
	held := w.Container
	if held == "" {
		held = w.Task
	}
	for _, p := range prs {
		if p.Agent == w.Name && p.Kind == "interim" && p.Task == held {
			return p.ID
		}
	}
	return ""
}
