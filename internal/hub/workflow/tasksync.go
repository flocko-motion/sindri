// package: hub/workflow / tasksync
// type:    logic (task cache sync + parent validation)
// job:     refresh the cached task set from its sources; validate a parent before write.
// limits:  read-only against the sources; writes only the cache table.
package workflow

import (
	"fmt"
	"github.com/flo-at/sindri/internal/api"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// SyncTasks refreshes a project's cached task set from its sources (td + openspec +
// the TTL-throttled GitHub scan). ForceSyncTasks bypasses the GitHub TTL for an
// explicit [r]efresh.
func (e *Engine) SyncTasks(project string) error { return e.syncTasks(project, false) }

// ForceSyncTasks is SyncTasks with the GitHub scan forced past its TTL ([r]efresh).
func (e *Engine) ForceSyncTasks(project string) error { return e.syncTasks(project, true) }

func (e *Engine) syncTasks(project string, force bool) error {
	root := e.deps.ProjectRoot(project)
	ps := e.store.For(project)
	// Before reading any source: a repo arriving with a td backlog gets it once, or its tasks
	// would simply be absent from the moment td stopped being a source.
	if err := e.importTdOnce(project, root); err != nil {
		return err
	}
	var rows []store.Task

	// Every source treated identically — the hub never branches on which it is. Each self-gates,
	// normalizes to task.Task, and throttles internally. td errors fail the sync (it is primary);
	// a network source degrades to its last good list.
	for _, src := range e.taskSources(project) {
		if !src.Enabled(root) {
			continue
		}
		ts, err := src.Tasks(root, force)
		if err != nil {
			return err
		}
		for _, t := range ts {
			rows = append(rows, ToStoreTask(t))
		}
	}

	if ov, err := ps.PriorityOverrides(); err == nil {
		for i := range rows {
			if p, ok := ov[rows[i].ID]; ok {
				rows[i].Priority = p
			}
		}
	}
	// Parentage is the hub's alone — no source carries it, so it is applied after the sources.
	if links, err := ps.ParentLinks(); err == nil {
		for i := range rows {
			if parent, ok := links[rows[i].ID]; ok {
				rows[i].ParentID = parent
			}
		}
	}
	return ps.ReplaceTasks(rows)
}

// checkParent validates a requested parent before anything is written: it must exist, and it must
// not already sit below the task being re-parented. A loop is unreachable from any root, so the task
// list would simply stop showing every task inside it.
func (e *Engine) checkParent(project, parent, self string) error {
	if parent == "" {
		return nil
	}
	if parent == self {
		return fmt.Errorf("a task can't be its own parent")
	}
	ps := e.store.For(project)
	tasks, err := ps.AllTasks()
	if err != nil {
		return err
	}
	known := false
	for _, t := range tasks {
		if t.ID == parent {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("unknown parent %q", parent)
	}
	if self == "" {
		return nil // a task being created has nothing below it yet
	}
	// Walk up from the proposed parent; reaching self means self is already an ancestor.
	links, err := ps.ParentLinks()
	if err != nil {
		return err
	}
	chain := []string{parent}
	for at := links[parent]; at != ""; at = links[at] {
		if at == self {
			return fmt.Errorf("%s already sits above %s (%s) — parenting it there would close a loop, "+
				"and everything inside a loop drops off the task list", self, parent,
				strings.Join(append(chain, self), " → "))
		}
		chain = append(chain, at)
		if len(chain) > len(links)+1 {
			return fmt.Errorf("the parent chain above %q doesn't terminate — a loop is already stored (%s)",
				parent, strings.Join(chain, " → "))
		}
	}
	return nil
}

// ToStoreTask maps a source-normalized domain task onto the hub's cached store row.
// Exported because the hub's targeted single-task refresh reuses the same mapping.
func ToStoreTask(t task.Task) store.Task {
	var updatedAt, createdAt string
	if !t.UpdatedAt.IsZero() {
		updatedAt = t.UpdatedAt.UTC().Format(time.RFC3339)
	}
	// Left empty when the source has no answer: the store keeps what it had, rather than aging every
	// task from the last sync.
	if !t.CreatedAt.IsZero() {
		createdAt = t.CreatedAt.UTC().Format(time.RFC3339)
	}
	return store.Task{
		ID: t.ID, Title: t.Title, Status: t.Status, Priority: t.Priority, Tier: t.Tier,
		Type: t.Type, Labels: strings.Join(t.Labels, ","), ParentID: t.ParentID,
		Description: t.Description, URL: t.URL, UpdatedAt: updatedAt, CreatedAt: createdAt,
	}
}

// checkTier refuses a word that is not a tier. Here rather than in a front-end, so every caller is
// held to it — and REFUSED rather than defaulted: TierOrDefault answers "mid" for anything it does
// not know, so a dropped or mistyped tier arrived as a deliberate-looking mid with nothing said.
func checkTier(tier string) error {
	if tier == "" {
		return nil // unset is a real answer: the task takes the default (-> api.TierOrDefault)
	}
	if _, ok := api.ParseTier(tier); !ok {
		return fmt.Errorf("unknown tier %q — one of: %s", tier, strings.Join(api.TierWords, ", "))
	}
	return nil
}
