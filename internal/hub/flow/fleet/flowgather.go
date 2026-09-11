// package: hub/flow/fleet / flowgather
// type:    logic (reading the whole world for one agent, once)
// job:     assemble the world every condition reads — the agent's situation, the verdict on what it
// holds, what the backlog would hand it, and whether its tree has been split under it.
// limits:  reading. The conditions are hub/flow/cond's, and acting is flowdo.go's.
package fleet

import (
	"fmt"
	"github.com/flo-at/sindri/internal/hub/world/task"
	"strings"

	"github.com/flo-at/sindri/internal/api"

	"github.com/flo-at/sindri/internal/hub/flow"
	flowpr "github.com/flo-at/sindri/internal/hub/flow/pr"
	"github.com/flo-at/sindri/internal/hub/world/situation"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// subject splits "project/agent", the identity the machine watches an agent by.
func subject(s string) (project, agent string, err error) {
	project, agent, ok := strings.Cut(s, "/")
	if !ok {
		return "", "", fmt.Errorf("flow subject %q is not project/agent", s)
	}
	return project, agent, nil
}

// gatherSubject reads the world for one "project/agent".
func (e *Engine) gatherSubject(s string) (flow.World, error) {
	project, agent, err := subject(s)
	if err != nil {
		return flow.World{}, err
	}
	return e.gather(project, agent)
}

// gather reads everything the conditions may look at, in one pass. Conditions reaching for what they
// need separately can decide from a world that never existed, so this is a deliberate step: after it
// returns, nothing else is read until the state has been judged.
func (e *Engine) gather(project, name string) (flow.World, error) {
	ps := e.Store.For(project)
	sit, err := e.Sit.Of(project, name)
	if err != nil {
		return flow.World{}, err
	}
	w := flow.World{Situation: sit}
	w.Aim, w.Ceiling = e.taskAct().CommentBudget(project)
	w.Unread, _ = ps.UnreadMailCount(name)
	if w.SplitTree, err = e.splitTree(ps, name, sit.Container); err != nil {
		return flow.World{}, err
	}
	// A feature's PR is filed against the FEATURE, never the subtask in hand, so that is what a
	// verdict on the work in hand stands against.
	target := sit.Container
	if target == "" {
		target = sit.Task
	}
	if w.Held, err = e.verdictOn(project, name, target); err != nil {
		return flow.World{}, err
	}
	if w.Awaiting, err = e.awaitingVerdict(ps, project, sit); err != nil {
		return flow.World{}, err
	}
	if err = e.gatherSubmit(ps, name, &w); err != nil {
		return flow.World{}, err
	}
	w.Conflicted = flowpr.ConflictStanding(ps, sit.AwaitingPR)
	w.GainedChildren = e.gainedChildren(ps, sit)
	w.MilestoneLanded = e.milestoneLanded(ps, sit)
	// FOUND AND CLOSED, never merely absent. A cache that has not synced, or a source that was
	// unreachable, both read as "not found" — and freeing every agent whose task the hub cannot
	// currently see is the failure mode this direction refuses to have.
	if target != "" {
		if t, found, terr := ps.GetTask(target); terr == nil && found {
			w.TaskGone = !api.Open(t)
		}
	}
	return w, e.gatherWork(ps, project, name, &w)
}

// gatherSubmit reads the submit an author has asked for and the question standing against it. Rows
// rather than a git command: whether the TREE still matches is asked at the two moments it changes
// anything, and never here — a git invocation per agent per pass is what the observer exists to
// prevent (-> flowinterview.go).
func (e *Engine) gatherSubmit(ps *store.ProjectStore, name string, w *flow.World) error {
	_, asked, err := ps.SubmitAsked(name)
	if err != nil {
		return err
	}
	w.SubmitAsked = asked
	if w.GateRefused, err = ps.LandingGateRefused(name); err != nil {
		return err
	}
	_, rows, err := ps.OpenSubmitAnswers(name)
	if err != nil {
		return err
	}
	_, w.InterviewQuestion, _, _ = flowpr.Standing(rows)
	return nil
}

// gatherWork fills in what the agent could be handed next — its feature's open children and the ones
// still gated, or the best-rated unit in the backlog when it holds no feature.
func (e *Engine) gatherWork(ps *store.ProjectStore, project, name string, w *flow.World) error {
	if w.Container != "" {
		children, err := ps.OpenSubtasks(w.Container)
		if err != nil {
			return err
		}
		w.Subtasks, w.Gated = children, w.Pool.GatedUnder(w.Container)
		if len(children) > 0 {
			w.TierMismatch = e.tierMismatch(w, children[0].Tier)
		}
		return nil
	}
	if w.Role == "reviewer" {
		var id int64
		var prID string
		// Fleet-wide, not this project's alone: a pooled reviewer's own project holds no pull requests,
		// so asking it answers "nothing waiting" while a repo's review sits unread.
		_, found, ferr := e.Store.UnclaimedReview(project, &id, &prID)
		w.ReviewWaiting = ferr == nil && found
		w.Wanted = w.ReviewWaiting
		return e.gatherPool(ps, w)
	}
	if w.Role != "worker" {
		return nil
	}
	spoken, err := e.spokenFor(ps, name)
	if err != nil {
		return err
	}
	packages, leaves := without(w.Pool.Packages, spoken), without(w.Pool.Leaves, spoken)
	w.Next, w.NextIsFeature, w.HasNext = task.NextUp(packages, leaves, e.roleAct().TierPrefers(project, name))
	w.Wanted = w.HasNext
	if w.HasNext {
		w.TierMismatch = e.tierMismatch(w, w.Next.Tier)
	}
	return e.gatherPool(ps, w)
}

// tierMismatch reports work rated for a model other than the one under the agent. Read here because
// which model a tier deserves is POLICY and whether two ids name one is the backend's own knowledge.
func (e *Engine) tierMismatch(w *flow.World, tier string) bool {
	want, known := e.Deps.ModelForTier(api.TierOrDefault(tier))
	return known && !e.Harness.ModelMatches(want, w.Model)
}

// gatherPool answers the two fleet-shaped questions a stopped agent cannot ask for itself: would
// anybody awake take this, and is this the one asleep to bring back. Per agent, so one wake, one pod.
func (e *Engine) gatherPool(ps *store.ProjectStore, w *flow.World) error {
	if !w.Wanted || !w.Stopped {
		return nil // nothing waiting, or this pod is not one to bring back
	}
	roster, err := ps.Roster()
	if err != nil {
		return err
	}
	first := ""
	for _, a := range roster {
		if a.Role != w.Role {
			continue // one role's queue never wakes another's
		}
		sit, serr := e.Sit.Of(w.Project, a.Name)
		if serr != nil {
			return serr
		}
		if sit.Allowed().Wake != "" {
			continue // it may not be pushed anything at all, so it answers for nothing here
		}
		// NOT-STOPPED rather than up: a pod coming back for this very row reads as down until the
		// observer catches up, and waking a second one in that window brings a whole pool back.
		if !sit.Stopped && sit.HoldsNothing() {
			w.PoolCovered = true
			return nil
		}
		if sit.Stopped && !sit.Up && first == "" {
			first = a.Name
		}
	}
	w.FirstAsleep = first == w.Name
	return nil
}

// spokenFor is every task another agent already holds a claim on. Kept out of this agent's pool so
// two going idle together are not handed the same one.
func (e *Engine) spokenFor(ps *store.ProjectStore, self string) (map[string]bool, error) {
	roster, err := ps.Roster()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, a := range roster {
		if a.Name == self {
			continue
		}
		if st, serr := ps.GetState(a.Name); serr == nil {
			if st.Task != "" {
				out[st.Task] = true
			}
			if st.Container != "" {
				out[st.Container] = true
			}
		}
	}
	return out, nil
}

// without drops the tasks somebody else already holds, preserving order.
func without(tasks []store.Task, spoken map[string]bool) []store.Task {
	if len(spoken) == 0 {
		return tasks
	}
	out := make([]store.Task, 0, len(tasks))
	for _, t := range tasks {
		if !spoken[t.ID] {
			out = append(out, t)
		}
	}
	return out
}

// splitTree reports another agent working inside the tree this one holds the container of.
func (e *Engine) splitTree(ps *store.ProjectStore, name, container string) (bool, error) {
	if container == "" {
		return false, nil
	}
	inside, err := ps.HeldDescendant(container)
	if err != nil {
		return false, err
	}
	return inside != "" && inside != name, nil
}

// verdictOn is the rejection standing against one piece of work, and how many rounds it has taken.
func (e *Engine) verdictOn(project, agent, target string) (flow.Verdict, error) {
	feedback, rejected, err := e.taskAct().PrRejected(project, agent, target)
	if err != nil || !rejected {
		return flow.Verdict{}, err
	}
	return flow.Verdict{Rejected: true, Feedback: feedback, Round: e.taskAct().RejectionRound(project, target)}, nil
}

// awaitingVerdict is the verdict on a PR the agent has out while its state row holds nothing else —
// the only thing still tying it to work its state was cleared of.
func (e *Engine) awaitingVerdict(ps *store.ProjectStore, project string, sit situation.Situation) (flow.Verdict, error) {
	if sit.AwaitingPR == "" {
		return flow.Verdict{}, nil
	}
	p, ok, err := ps.GetPR(sit.AwaitingPR)
	if err != nil || !ok || p.Status != "rejected" {
		return flow.Verdict{}, err
	}
	return flow.Verdict{Rejected: true, Feedback: p.Feedback,
		Round: e.taskAct().RejectionRound(project, sit.AwaitingTask)}, nil
}

// gainedChildren reports a LEAF task that has grown children while its holder worked it. A task that
// gained a child is a feature, and an agent holding one must take the new work on rather than be
// stranded in front of it or merge over it.
func (e *Engine) gainedChildren(ps *store.ProjectStore, sit situation.Situation) bool {
	if sit.Container != "" || sit.Task == "" {
		return false
	}
	open, err := ps.OpenChildIDs(sit.Task)
	return err == nil && len(open) > 0
}

// milestoneLanded reports an interim pull request of this agent's that merged while it still holds
// the work: its standing branch is behind the base that merge moved. Interim only — a FINAL merge
// ends the task, and there is no branch left to catch up.
func (e *Engine) milestoneLanded(ps *store.ProjectStore, sit situation.Situation) bool {
	if sit.Task == "" && sit.Container == "" {
		return false
	}
	prs, err := ps.PRs("merged")
	if err != nil {
		return false
	}
	held := sit.Container
	if held == "" {
		held = sit.Task
	}
	for _, p := range prs {
		if p.Agent == sit.Name && p.Kind == "interim" && p.Task == held && !caughtUp(ps, p.ID) {
			return true
		}
	}
	return false
}

// caughtUp reports an author already brought onto the base its own milestone moved. Without it the
// condition stays true for as long as the merged PR exists, and the agent resets its branch onto the
// same base on every beat — a loop whose only symptom is work being reapplied for ever.
func caughtUp(ps *store.ProjectStore, prID string) bool {
	events, err := ps.PREvents(prID)
	if err != nil {
		return true // unreadable: do not reset a branch on a guess
	}
	for _, e := range events {
		if e.Type == "resumed" {
			return true
		}
	}
	return false
}
