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
	return ps.UpsertTask(store.Task{
		ID: owned.ID, Title: owned.Title, Status: owned.Status, Priority: owned.Priority,
		Type: owned.Type, Labels: owned.Labels, ParentID: ps.ParentOf(id),
		Description: owned.Description, UpdatedAt: owned.UpdatedAt,
	})
}

// refreshCachedTask updates one task's cached row after a local mutation, instead
// of a full multi-source SyncTasks: a task sindri owns is re-read from its own table; a gh-/os- one
// keeps its synced fields and has the hub's own — priority, parent — laid back over them, since a
// local edit changes those and not what the source holds.
// Best-effort: a failure is logged host-side, never surfaced to the mutation.
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
	// The parent too, for the reason the priority is here: both are the hub's, so a targeted refresh
	// that skipped one showed a re-parented task as a root until some later full sync.
	t.ParentID = ps.ParentOf(id)
	if err := ps.UpsertTask(t); err != nil {
		fmt.Fprintf(os.Stderr, "hub: refresh cached task %s: %v\n", id, err)
	}
}

// reconciledStatus is the pure rule: a task said to be under review with no open PR isn't, one said
// to be in progress with no assigned agent isn't, and one said to be DONE with open work under it
// isn't either — a parent is finished exactly when its children are. Everything else is left as-is.
// Returns the status the task should have.
func reconciledStatus(status string, activePR, assigned, openChildren bool) string {
	if openChildren && (task.Task{Status: status}).IsClosed() {
		return "open" // reopened rather than left lying: the work beneath it is real and unfinished
	}
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

// taskReality reports whether a task currently has an active (not merged/rejected)
// PR and whether any agent is assigned to it — the two facts reconciledStatus needs.
func (e *Engine) taskReality(project, id string) (activePR, assigned bool, err error) {
	ps := e.store.For(project)
	prs, err := ps.PRs()
	if err != nil {
		return false, false, err
	}
	for _, p := range prs {
		if p.Task == id && p.Status != "merged" && p.Status != "rejected" {
			activePR = true
			break
		}
	}
	roster, err := ps.Roster()
	if err != nil {
		return false, false, err
	}
	for _, a := range roster {
		if st, _ := ps.GetState(a.Name); st.Task == id {
			assigned = true
			break
		}
	}
	return activePR, assigned, nil
}

// ReconcileTask repairs one td task's status against reality (a no-op for gh-/os-
// ids and for a task that's already consistent). Writes the correction to td so it
// persists through the next sync.
func (e *Engine) ReconcileTask(project, id string) error {
	ps := e.store.For(project)
	live, ok, err := ps.OwnedTask(id)
	if err != nil || !ok {
		return err // an id owned elsewhere carries its own status; nothing here to repair
	}
	activePR, assigned, err := e.taskReality(project, id)
	if err != nil {
		return err
	}
	open, err := ps.OpenChildIDs(id)
	if err != nil {
		return err
	}
	want := reconciledStatus(live.Status, activePR, assigned, len(open) > 0)
	if want == live.Status {
		return nil
	}
	if err := ps.SetOwnedStatus(id, want); err != nil {
		return err
	}
	return e.RefreshTask(project, id)
}

// ReconcileTasks repairs every td task in a project in one pass (the task-list /
// TUI-startup sweep). A per-task failure is logged, never fatal to the sweep.
func (e *Engine) ReconcileTasks(project string) error {
	ps := e.store.For(project)
	tasks, err := ps.OwnedTasks()
	if err != nil {
		return err
	}
	prs, err := ps.PRs()
	if err != nil {
		return err
	}
	activePR := map[string]bool{}
	for _, p := range prs {
		if p.Status != "merged" && p.Status != "rejected" {
			activePR[p.Task] = true
		}
	}
	assigned := map[string]bool{}
	roster, err := ps.Roster()
	if err != nil {
		return err
	}
	for _, a := range roster {
		if st, _ := ps.GetState(a.Name); st.Task != "" {
			assigned[st.Task] = true
		}
	}
	// Which tasks still have open work under them, from the cache in one pass rather than a query
	// per task: the sweep runs at every task list and TUI start.
	all, err := ps.AllTasks()
	if err != nil {
		return err
	}
	hasOpenChild := map[string]bool{}
	for _, t := range all {
		if t.ParentID != "" && t.Status == "open" {
			hasOpenChild[t.ParentID] = true
		}
	}
	changed := false
	for _, t := range tasks {
		if want := reconciledStatus(t.Status, activePR[t.ID], assigned[t.ID], hasOpenChild[t.ID]); want != t.Status {
			if err := ps.SetOwnedStatus(t.ID, want); err != nil {
				fmt.Fprintf(os.Stderr, "hub: reconcile %s (%s->%s): %v\n", t.ID, t.Status, want, err)
				continue
			}
			_ = e.RefreshTask(project, t.ID)
			changed = true
		}
	}
	if changed {
		e.deps.Notify()
	}
	return nil
}
