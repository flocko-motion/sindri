// package: hub/workflow / reconcile
// type:    logic (task-status repair)
// job:     correct a task's stored status against reality — "in_review" with no open
// PR, "in_progress" with no assignee, or "closed" over open subtasks is stale.
// Repairs the owning store so it heals, at task list / info / TUI startup.
// limits:  owned tasks only; one write per real discrepancy, then a no-op.
package workflow

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// refreshTask re-reads one task from td and updates its cached row — the targeted
// alternative to a full SyncTasks after a single-task change.
func (e *Engine) RefreshTask(project, id string) error {
	ps := e.store.For(project)
	owned, ok, err := ps.OwnedTask(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("refresh %s: this project owns no such task", id)
	}
	return ps.UpsertTask(ownedToCachedTask(owned, ps.ParentOf(id)))
}

// ownedToCachedTask projects an owned task onto the cached row every source shares (store.Task).
// The ONE place that does: a second hand-written copy is how a field added here drifted from there.
func ownedToCachedTask(owned store.OwnedTask, parentID string) store.Task {
	return store.Task{
		ID: owned.ID, Title: owned.Title, Status: owned.Status, Priority: owned.Priority, Tier: owned.Tier,
		Type: owned.Type, Labels: owned.Labels, ParentID: parentID,
		Description: owned.Description, UpdatedAt: owned.UpdatedAt,
	}
}

// refreshCachedTask updates one task's cached row after a local mutation, sparing a full
// multi-source SyncTasks: an owned task is re-read from its own table, a gh-/os- one keeps its
// synced fields under the hub's own overrides. Best-effort, logged host-side.
func (e *Engine) refreshCachedTask(project, id string) {
	ps := e.store.For(project)
	if ps.OwnsTask(id) {
		if err := e.RefreshTask(project, id); err != nil {
			fmt.Fprintf(os.Stderr, "hub: refresh task %s: %v\n", id, err)
		}
		return
	}
	t, ok, err := ps.GetTask(id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hub: refresh cached task %s: %v\n", id, err)
		return
	}
	if !ok {
		return
	}
	if ov, oerr := ps.PriorityOverrides(); oerr == nil {
		t.Priority = ov[id]
	}
	if ov, oerr := ps.TierOverrides(); oerr == nil {
		if tier, set := ov[id]; set {
			t.Tier = tier
		}
	}
	// The parent too, for the reason the priority is here: both are the hub's, so a targeted refresh
	// that skipped one showed a re-parented task as a root until some later full sync.
	t.ParentID = ps.ParentOf(id)
	if err := ps.UpsertTask(t); err != nil {
		fmt.Fprintf(os.Stderr, "hub: refresh cached task %s: %v\n", id, err)
	}
}

// taskFacts is what the sweep knows about one task's reality — the four things reconciledStatus
// weighs against what the task claims about itself.
type taskFacts struct {
	activePR      bool // a PR neither merged nor rejected: the task really is out for review
	assigned      bool // an agent holds it
	openChildren  bool // work remains beneath it
	mergedFinalPR bool // its work has landed; an interim contribution does NOT count (the task goes on)
}

// reconciledStatus is the pure rule, weighing what a task claims against taskFacts and returning the
// status it should have. Everything it has no opinion on is left alone.
func reconciledStatus(status string, f taskFacts) string {
	if f.openChildren && (task.Task{Status: status}).IsClosed() {
		return "open" // reopened rather than left lying: the work beneath it is real and unfinished
	}
	// Landed work outranks a stale "open": a merge is the end of a task, so a tree left open by one
	// that took the wrong path is closed here rather than waiting on someone to notice it.
	if f.mergedFinalPR && !f.openChildren && !(task.Task{Status: status}).IsClosed() {
		return "closed"
	}
	activePR, assigned := f.activePR, f.assigned
	switch status {
	case "in_review":
		if !activePR {
			if assigned {
				return "in_progress"
			}
			return "open"
		}
	case "in_progress":
		if !assigned {
			return "open"
		}
	}
	return status
}

// taskReality gathers what is actually true of one task, for reconciledStatus to judge its claim
// against.
func (e *Engine) taskReality(project, id string) (taskFacts, error) {
	ps := e.store.For(project)
	var f taskFacts
	prs, err := ps.PRs()
	if err != nil {
		return f, err
	}
	for _, p := range prs {
		if p.Task != id {
			continue
		}
		switch {
		case p.Status == "merged" && p.Kind != "interim":
			f.mergedFinalPR = true
		case p.Status != "merged" && p.Status != "rejected":
			f.activePR = true
		}
	}
	roster, err := ps.Roster()
	if err != nil {
		return f, err
	}
	for _, a := range roster {
		if st, _ := ps.GetState(a.Name); st.Task == id {
			f.assigned = true
			break
		}
	}
	open, err := ps.OpenChildIDs(id)
	if err != nil {
		return f, err
	}
	f.openChildren = len(open) > 0
	return f, nil
}

// ReconcileTask repairs one task's status against reality, for any task: SetStatus knows where each
// kind's status lives, so this does not. Repairing only the ones sindri owned is what left five
// openspec changes open behind merged PRs, invisible to the sweep that existed to catch exactly that.
func (e *Engine) ReconcileTask(project, id string) error {
	ps := e.store.For(project)
	live, ok, err := ps.GetTask(id)
	if err != nil || !ok {
		return err
	}
	facts, err := e.taskReality(project, id)
	if err != nil {
		return err
	}
	want := reconciledStatus(live.Status, facts)
	if want == live.Status {
		return nil
	}
	if err := e.SetStatus(project, id, want); err != nil {
		return err
	}
	return e.RefreshTask(project, id)
}

// ReconcileTasks repairs EVERY task in a project in one pass (the task-list / TUI-startup sweep),
// whatever source it came from. A per-task failure is logged, never fatal to the sweep.
func (e *Engine) ReconcileTasks(project string) error {
	ps := e.store.For(project)
	tasks, err := ps.AllTasks()
	if err != nil {
		return err
	}
	prs, err := ps.PRs()
	if err != nil {
		return err
	}
	// The same facts taskReality gathers per task, collected once for the whole project: the sweep
	// runs at every task list and TUI start, so a query per task would be paid on every one of them.
	facts := map[string]*taskFacts{}
	factsFor := func(id string) *taskFacts {
		if f := facts[id]; f != nil {
			return f
		}
		f := &taskFacts{}
		facts[id] = f
		return f
	}
	for _, p := range prs {
		switch {
		case p.Status == "merged" && p.Kind != "interim":
			factsFor(p.Task).mergedFinalPR = true
		case p.Status != "merged" && p.Status != "rejected":
			factsFor(p.Task).activePR = true
		}
	}
	roster, err := ps.Roster()
	if err != nil {
		return err
	}
	for _, a := range roster {
		if st, _ := ps.GetState(a.Name); st.Task != "" {
			factsFor(st.Task).assigned = true
		}
	}
	for _, t := range tasks {
		if t.ParentID != "" && t.Status == "open" {
			factsFor(t.ParentID).openChildren = true
		}
	}
	changed := false
	for _, t := range tasks {
		if want := reconciledStatus(t.Status, *factsFor(t.ID)); want != t.Status {
			if err := e.SetStatus(project, t.ID, want); err != nil {
				fmt.Fprintf(os.Stderr, "hub: reconcile %s (%s->%s): %v\n", t.ID, t.Status, want, err)
				continue
			}
			_ = e.RefreshTask(project, t.ID)
			changed = true
		}
	}
	if e.scrapPRsOnClosedTasks(project, prs, tasks) {
		changed = true
	}
	if e.HealSplitHierarchies(project) {
		changed = true
	}
	if changed {
		e.deps.Notify()
	}
	return nil
}

// scrapPRsOnClosedTasks settles a live PR whose task has closed: it can never land, so leaving it
// open made every reader carry the exception. AwaitingPR excludes it by hand and openPRFor does
// not, so hepti showed a PR on a task closed a week earlier while the hold rule said otherwise.
func (e *Engine) scrapPRsOnClosedTasks(project string, prs []store.PR, tasks []store.Task) (scrapped bool) {
	closed := map[string]bool{}
	for _, t := range tasks {
		if api.DoneStatus(t.Status) {
			closed[t.ID] = true
		}
	}
	ps := e.store.For(project)
	for _, p := range prs {
		if !api.PROpen(p) || p.Task == "" || !closed[p.Task] {
			continue
		}
		p.Status = "scrapped"
		p.Feedback = "the task closed while this was up, so there is nothing left for it to land into"
		if err := ps.PutPR(p); err != nil {
			fmt.Fprintf(os.Stderr, "hub: scrapping %s over closed %s: %v\n", p.ID, p.Task, err)
			continue
		}
		_ = ps.LogPR(p.ID, "scrapped", "its task "+p.Task+" is closed")
		_ = ps.Log(p.Agent, "pr-scrapped", p.ID+": "+p.Task+" is closed")
		scrapped = true
	}
	return scrapped
}

// HealSplitHierarchies frees every container holder whose tree somebody else is already working —
// the claim guard cannot cover a tree SPLIT after the fact by reparenting (-> healSplit).
func (e *Engine) HealSplitHierarchies(project string) (moved bool) {
	roster, err := e.store.For(project).Roster()
	if err != nil {
		return false
	}
	for _, a := range roster {
		if e.healSplit(project, a.Name) {
			moved = true
		}
	}
	return moved
}

// healSplit frees ONE container holder whose tree another agent is inside, so the hub can ask
// wherever it reads state: the sweep above, and every ask for work (-> directive). The CONTAINER
// holder yields, since the leaf is the concrete work — sudri held sd-ca28d3 while dvalin was a day
// into the subtask holding it open.
func (e *Engine) healSplit(project, name string) bool {
	ps := e.store.For(project)
	st, err := ps.GetState(name)
	if err != nil || st.Container == "" {
		return false
	}
	held, herr := ps.HeldDescendant(st.Container)
	if herr != nil || held == "" || held == name {
		return false
	}
	a, _, _ := ps.GetAgent(name)
	if serr := ps.SetState(store.AgentState{Agent: name, Phase: restPhase(a.Role)},
		store.ReasonFreed, "yielded "+st.Container+" to "+held); serr != nil {
		return false
	}
	// The PR goes with the feature. Left standing it binds the agent to a tree it no longer holds:
	// AwaitingPR treats an unsettled PR as held work, so the directive kept sending sudri back to
	// sd-ca28d3 while `sindri task` told it — correctly — that it held nothing.
	e.settleReleasedPR(ps, project, name, st.Container, held)
	_ = ps.Log(name, "container-released", st.Container+": "+held+" is working inside it")
	_ = e.deps.Deliver(project, name, MsgHierarchyTaken(st.Container, held), MailAndPush)
	return true
}

// settleReleasedPR closes an agent's unsettled PR against a feature taken off it — scrapped, since
// nobody is going to land a branch for a tree somebody else now owns. The branch is untouched.
func (e *Engine) settleReleasedPR(ps *store.ProjectStore, project, name, container, held string) {
	prs, err := ps.PRs()
	if err != nil {
		return
	}
	for _, pr := range prs {
		if pr.Agent != name || pr.Task != container || !api.PROpen(pr) {
			continue
		}
		pr.Status, pr.Feedback = "scrapped", "the feature went to "+held+", who is working inside it"
		if perr := ps.PutPR(pr); perr != nil {
			continue
		}
		e.releaseReviewers(project, pr.ID, "its feature changed hands")
		_ = ps.LogPR(pr.ID, "scrapped", "released with "+container)
	}
}
