// package: hub/flow/task / reconcile_act
// type:    logic (task-status repair)
// job:     correct a task's stored status against reality — "in_review" with no open
// PR, "in_progress" with no assignee, or "closed" over open subtasks is stale.
// Repairs the owning store so it heals, at task list / info / TUI startup.
// limits:  owned tasks only; one write per real discrepancy, then a no-op.
package task

import (
	"fmt"
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"os"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// refreshTask re-reads one task from td and updates its cached row — the targeted
// alternative to a full SyncTasks after a single-task change.
func (a *Act) RefreshTask(project, id string) error {
	ps := a.Store.For(project)
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

// RefreshCachedTask updates one task's cached row after a local mutation, sparing a full
// multi-source SyncTasks: an owned task is re-read from its own table, a gh-/os- one keeps its
// synced fields under the hub's own overrides. Best-effort, logged host-side.
func (a *Act) RefreshCachedTask(project, id string) {
	ps := a.Store.For(project)
	if ps.OwnsTask(id) {
		if err := a.RefreshTask(project, id); err != nil {
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
	if ov, oerr := ps.ClosedOverrides(); oerr == nil && ov[id] {
		t.Status = "closed" // ended here, and its source cannot see that yet (-> SetClosedOverride)
	}
	// The parent too, for the reason the priority is here: both are the hub's, so a targeted refresh
	// that skipped one showed a re-parented task as a root until some later full sync.
	t.ParentID = ps.ParentOf(id)
	if err := ps.UpsertTask(t); err != nil {
		fmt.Fprintf(os.Stderr, "hub: refresh cached task %s: %v\n", id, err)
	}
}

// Reality is what one task's truth amounts to — the four things the task map's conditions weigh
// against what that task claims about itself (-> hub/flow/task).
type Reality struct {
	ActivePR      bool // a PR neither merged nor rejected: the task really is out for review
	Assigned      bool // an agent holds it
	OpenChildren  bool // work remains beneath it
	MergedFinalPR bool // its work has landed; an interim contribution does NOT count (the task goes on)
}

// TaskReality gathers what is actually true of one task, for the task map to judge its claim
// against.
func (a *Act) TaskReality(project, id string) (Reality, error) {
	ps := a.Store.For(project)
	var f Reality
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
			f.MergedFinalPR = true
		case p.Status != "merged" && p.Status != "rejected":
			f.ActivePR = true
		}
	}
	roster, err := ps.Roster()
	if err != nil {
		return f, err
	}
	for _, ag := range roster {
		if st, _ := ps.GetState(ag.Name); st.Task == id {
			f.Assigned = true
			break
		}
	}
	open, err := ps.OpenChildIDs(id)
	if err != nil {
		return f, err
	}
	f.OpenChildren = len(open) > 0
	return f, nil
}

// ReconcileTask settles one task's status against reality. The rules it used to apply by hand are
// the task map's own conditions now (-> hub/flow/task), so this only asks the machine to look.
func (a *Act) ReconcileTask(project, id string) error {
	a.Flow.LookTask(project, id)
	return nil
}

// ReconcileTasks settles EVERY active task in a project — the task-list and TUI-startup sweep —
// and clears up what the task map does not own: a live pull request against a task that has closed,
// and a tree two agents ended up inside.
func (a *Act) ReconcileTasks(project string) error {
	a.Flow.LookTasks(project)
	a.Flow.LookPRs(project) // a PR against a task that just closed is scrapped by its own map
	if a.HealSplitHierarchies(project) {
		a.Deps.Notify()
	}
	return nil
}

// HealSplitHierarchies frees every container holder whose tree somebody else is already working —
// the claim guard cannot cover a tree SPLIT after the fact by reparenting (-> HealSplit).
func (a *Act) HealSplitHierarchies(project string) (moved bool) {
	roster, err := a.Store.For(project).Roster()
	if err != nil {
		return false
	}
	for _, ag := range roster {
		if a.HealSplit(project, ag.Name) {
			moved = true
		}
	}
	return moved
}

// HealSplit frees ONE container holder whose tree another agent is inside, so the hub can ask
// wherever it reads state: the sweep above, and every ask for work (-> directive). The CONTAINER
// holder yields, since the leaf is the concrete work — sudri held sd-ca28d3 while dvalin was a day
// into the subtask holding it open.
func (a *Act) HealSplit(project, name string) bool {
	ps := a.Store.For(project)
	st, err := ps.GetState(name)
	if err != nil || st.Container == "" {
		return false
	}
	held, herr := ps.HeldDescendant(st.Container)
	if herr != nil || held == "" || held == name {
		return false
	}
	ag, _, _ := ps.GetAgent(name)
	if serr := ps.SetState(store.AgentState{Agent: name, Phase: core.RestPhase(ag.Role)},
		store.ReasonFreed, "yielded "+st.Container+" to "+held); serr != nil {
		return false
	}
	// The PR goes with the feature. Left standing it binds the agent to a tree it no longer holds:
	// AwaitingPR treats an unsettled PR as held work, so the directive kept sending sudri back to
	// sd-ca28d3 while `sindri task` told it — correctly — that it held nothing.
	a.settleReleasedPR(ps, project, name, st.Container, held)
	_ = ps.Log(name, "container-released", st.Container+": "+held+" is working inside it")
	_ = a.Harness.Say(project, name, prompts.MsgHierarchyTaken(st.Container, held), core.MailAndPush)
	return true
}

// settleReleasedPR closes an agent's unsettled PR against a feature taken off it — scrapped, since
// nobody is going to land a branch for a tree somebody else now owns. The branch is untouched.
func (a *Act) settleReleasedPR(ps *store.ProjectStore, project, name, container, held string) {
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
		a.pr().ReleaseReviewers(project, pr.ID, "its feature changed hands")
		_ = ps.LogPR(pr.ID, "scrapped", "released with "+container)
	}
}
