// package: hub/flow/fleet / flowdo
// type:    logic (the implementations behind the declared actions)
// job:     one function per action a state can run, registered against the identity the flow file
// names. This is the only place in the flow that writes anything or touches the harness.
// limits:  doing. WHICH action runs where is the flow's declaration (-> hub/flow/roles).
package fleet

import (
	"context"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"log"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
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
		act.Interview.Name:   e.doInterview,
		act.Submit.Name:      e.doSubmit,
		act.TakeReview.Name:  e.doTakeReview,
		act.DropReview.Name:  e.doDropReview,
		act.Prod.Name:        e.doProd,
		act.Promote.Name:     e.doPromote,
		act.Rebase.Name:      e.doRebase,
		act.Launch.Name:      e.doLaunch,
		act.Stop.Name:        e.doStop,
		act.Disown.Name:      e.doDisown,
	}
}

// doPickWork claims the best-rated unit the backlog would hand this agent and delivers its brief.
// The claim comes FIRST, so holding the work protects it while the preparation behind it runs — and
// so a pod that is down or mid-restart still HOLDS the work it was given: delivery reports whether
// the brief landed, and the agent is told what it holds on its next ask either way.
func (e *Engine) doPickWork(_ context.Context, w flow.World) (flow.Outcome, error) {
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
	// The session was prepared BEFORE this state was reached — a model switch, else a clear, each
	// its own state — so all that is left is to say what the agent now holds.
	return act.Done, e.Harness.Say(w.Project, w.Name, dir, mail.PushOnly)
}

// doPickSubtask moves a feature holder onto its feature's next open child.
func (e *Engine) doPickSubtask(_ context.Context, w flow.World) (flow.Outcome, error) {
	if len(w.Subtasks) == 0 {
		return act.Nothing, nil
	}
	child := w.Subtasks[0]
	if err := e.taskAct().StartSubtask(w.Project, w.Name, w.Container, child); err != nil {
		return act.Nothing, err
	}
	dir := prompts.DirContainerWorking(w.Container, child.ID, w.Aim, w.Ceiling)
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
	err := e.Store.For(w.Project).SetHolding(w.Name, "", "", "",
		store.ReasonLanded, "feature already landed: "+w.Container)
	return act.Done, err
}

// doSubmit takes what the agent has, unattended: the author answered for this tree before it got
// here, so nothing in it can ask a question. What it does is the PR subject's (-> pr.TakeSubmit);
// this says where each answer leaves the agent.
func (e *Engine) doSubmit(_ context.Context, w flow.World) (flow.Outcome, error) {
	qr, reused, err := e.prAct().TakeSubmit(w.Project, w.Name)
	if err != nil {
		// Back to the work rather than an error: the agent still holds it, and a submit that could not
		// be taken is something for it to try again rather than something to strand it over.
		_ = e.Store.For(w.Project).Log(w.Name, "submit-failed", err.Error())
		return act.Failed, nil
	}
	if reused {
		// The stored pass landed the pull request inside TakeSubmit, so there is nothing left to wait for.
		_ = e.Harness.Say(w.Project, w.Name, "[hub] "+prompts.ReplyGateReused(qr.ID, prompts.ShortSHA(qr.Commit)), mail.PushOnly)
		return act.Done, nil
	}
	_ = e.Harness.Say(w.Project, w.Name, "[hub] "+prompts.ReplyGateQueued(qr.ID, e.prAct().QueuePosition(qr.ID)), mail.PushOnly)
	return act.Queued, nil
}

// doLaunch brings a reclaimed pod back up, so the work waiting for this agent has somewhere to land.
// The request a human made is answered the moment this runs, whatever the launch then does: left
// standing, the condition that brought the agent here holds again the instant it leaves.
func (e *Engine) doLaunch(ctx context.Context, w flow.World) (flow.Outcome, error) {
	e.roleAct().AnswerPodRequest(w.Project, w.Name)
	if w.Up {
		return act.Done, nil // already running; whatever asked has been answered by somebody else
	}
	if err := e.Harness.Start(ctx, w.Project, w.Name); err != nil {
		_ = e.Store.For(w.Project).Log(w.Name, "launch", "failed: "+err.Error())
		return act.Failed, nil
	}
	// The flag goes with the launch, not with the observation: it means a pod somebody took down on
	// purpose, and it is no longer true the moment one is asked for. Left standing until the observer
	// caught up, the condition that brought the agent here would hold again on the very next pass.
	e.unpark(w.Project, w.Name)
	_ = e.Store.For(w.Project).Log(w.Name, "wake", "started for work its role answers for")
	return act.Done, nil
}

// unpark takes back the flag that says a pod was torn down on purpose.
func (e *Engine) unpark(project, name string) {
	ps := e.Store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil || !ok || !a.Stopped {
		return
	}
	a.Stopped = false
	if perr := ps.PutAgent(a); perr != nil {
		log.Printf("hub: clearing %s's stopped flag: %v", name, perr)
	}
}

// doStop takes an idle pod back. The session is preserved, so being wrong costs the next start's
// latency and nothing more — which is why idleness alone triggers this and memory pressure never does.
func (e *Engine) doStop(ctx context.Context, w flow.World) (flow.Outcome, error) {
	e.roleAct().AnswerPodRequest(w.Project, w.Name)
	if !w.Up {
		return act.Done, nil // nothing running to reclaim
	}
	if err := e.Harness.Stop(ctx, w.Project, w.Name); err != nil {
		_ = e.Store.For(w.Project).Log(w.Name, "stop", "did not land: "+err.Error())
		return act.Failed, nil
	}
	e.park(w.Project, w.Name)
	return act.Done, nil
}

// doDisown puts back a backlog task that ended up on a row whose role holds none, and frees the row.
func (e *Engine) doDisown(_ context.Context, w flow.World) (flow.Outcome, error) {
	held := w.Container
	if held == "" {
		held = w.Task
	}
	ps := e.Store.For(w.Project)
	if err := e.prAct().SetStatus(w.Project, held, "open"); err != nil {
		_ = ps.Log(w.Name, "unassign", held+": could not be put back — "+err.Error())
		return act.Failed, nil
	}
	if err := ps.SetHolding(w.Name, "", "", "",
		store.ReasonFreed, "a "+w.Role+" holds no backlog task: "+held); err != nil {
		return act.Failed, err
	}
	_ = ps.Log(w.Name, "unassign", held+" (a "+w.Role+" holds no backlog task)")
	e.WakeProject(w.Project, topic.TaskAvailable)
	return act.Done, nil
}

// park records a pod taken down on purpose, so "stopped" (resumable) reads distinct from "down"
// (crashed) even across a hub restart that drops the observer's own memory of it.
func (e *Engine) park(project, name string) {
	ps := e.Store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil || !ok || a.Stopped {
		return
	}
	a.Stopped = true
	if perr := ps.PutAgent(a); perr != nil {
		log.Printf("hub: recording %s as stopped: %v", name, perr)
	}
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
	claimed, err := e.prAct().AssignReview(ctx, home, id, prID, w.Name, req)
	if err != nil {
		return act.Failed, err
	}
	if !claimed {
		return act.Nothing, nil // another reviewer reached the row first
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
	// Through the one release, which is the only place a review is closed: a second path closing one
	// is how a reviewer came to be freed somewhere the machine could not see it.
	e.prAct().ReleaseReviewers(heldProject, held, "overtaken: the PR settled before a verdict")
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
