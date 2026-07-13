// package: hub / reconcile
// type:    logic (task-status repair)
// job:     correct a td task's stored status against reality — "in_review" with no
//
//	open PR, or "in_progress" with no assignee, is stale. Repairs td (the
//	source of truth) so it heals, at task list / info / TUI startup.
//
// limits:  td-* tasks only; one td write per real discrepancy, then a no-op.
package hub

import (
	"fmt"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/tasks/td"
)

// refreshTask re-reads one task from td and updates its cached row — the targeted
// alternative to a full SyncTasks after a single-task change.
func (h *Hub) refreshTask(project, id string) error {
	t, err := td.Get(h.projectRoot(project), id)
	if err != nil {
		return err
	}
	return h.store.For(project).UpsertTask(toStoreTask(t))
}

// refreshCachedTask updates one task's cached row after a local mutation, instead
// of a full multi-source SyncTasks (td + openspec + the GitHub scan): a td task is
// re-read from td; a gh-/os- task keeps its synced fields and just has its current
// priority override re-applied — its source fields don't change on a local edit.
// Best-effort: a failure is logged host-side, never surfaced to the mutation.
func (h *Hub) refreshCachedTask(project, id string) {
	if strings.HasPrefix(id, "td-") {
		if err := h.refreshTask(project, id); err != nil {
			fmt.Fprintf(os.Stderr, "hub: refresh task %s: %v\n", id, err)
		}
		return
	}
	ps := h.store.For(project)
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
	if err := ps.UpsertTask(t); err != nil {
		fmt.Fprintf(os.Stderr, "hub: refresh cached task %s: %v\n", id, err)
	}
}

// reconciledStatus is the pure rule: a task said to be under review with no open PR
// isn't, and one said to be in progress with no assigned agent isn't. Everything
// else is left as-is. Returns the status the task should have.
func reconciledStatus(status string, activePR, assigned bool) string {
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
func (h *Hub) taskReality(project, id string) (activePR, assigned bool, err error) {
	ps := h.store.For(project)
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
func (h *Hub) ReconcileTask(project, id string) error {
	if !strings.HasPrefix(id, "td-") {
		return nil
	}
	ps := h.store.For(project)
	t, ok, err := ps.GetTask(id)
	if err != nil || !ok {
		return err
	}
	activePR, assigned, err := h.taskReality(project, id)
	if err != nil {
		return err
	}
	want := reconciledStatus(t.Status, activePR, assigned)
	if want == t.Status {
		return nil
	}
	if err := td.SetStatus(h.projectRoot(project), id, want); err != nil {
		return err
	}
	return h.refreshTask(project, id)
}

// ReconcileTasks repairs every td task in a project in one pass (the task-list /
// TUI-startup sweep). A per-task failure is logged, never fatal to the sweep.
func (h *Hub) ReconcileTasks(project string) error {
	ps := h.store.For(project)
	tasks, err := ps.AllTasks()
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
	changed := false
	for _, t := range tasks {
		if !strings.HasPrefix(t.ID, "td-") {
			continue
		}
		if want := reconciledStatus(t.Status, activePR[t.ID], assigned[t.ID]); want != t.Status {
			if err := td.SetStatus(h.projectRoot(project), t.ID, want); err != nil {
				fmt.Fprintf(os.Stderr, "hub: reconcile %s (%s->%s): %v\n", t.ID, t.Status, want, err)
				continue
			}
			_ = h.refreshTask(project, t.ID)
			changed = true
		}
	}
	if changed {
		h.notify()
	}
	return nil
}
