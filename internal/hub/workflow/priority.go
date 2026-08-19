// package: hub/workflow / priority
// type:    logic (rating a task, and how far the rating reaches)
// job:     assign a task's priority and write it where that task's priority lives — its own
// row when sindri owns the task, the hub's overlay when the task is mirrored — over
// as much of the subtree as the scope asks for.
// limits:  the write and its reach; what a scoped rating MEANS is api.PriorityEffect's, and
// who may rate is the command surface's.
package workflow

import (
	"github.com/flo-at/sindri/internal/api"
)

// SetPriority assigns a task's priority (a P-code), reaching as far below it as scope asks. What
// reaching there does differs by case, and api.PriorityEffect is where that is set out.
func (e *Engine) SetPriority(project, id, priority string, scope api.PriorityScope) error {
	targets, err := e.priorityTargets(project, id, scope)
	if err != nil {
		return err
	}
	for _, t := range targets {
		if err := e.writePriority(project, t, priority); err != nil {
			return err
		}
		e.refreshCachedTask(project, t) // targeted refresh of each reprioritized task
	}
	e.deps.Notify()
	// Rating an unrated task is the moment it becomes claimable, so it needs the same nudge as a task
	// created with a priority. ONE, however far the cascade reached: it only has to wake a worker up.
	e.nudgeIdleWorkers(project, priority)
	return nil
}

// priorityTargets is which tasks a scoped rating writes to, CHILDREN FIRST — the parent's rating is
// what releases a package, so no worker can claim one half-rated. Open descendants only: a finished
// task's rating decides nothing, and overwriting it would edit the record of work already done.
func (e *Engine) priorityTargets(project, id string, scope api.PriorityScope) ([]string, error) {
	if scope == api.ScopeTask {
		return []string{id}, nil
	}
	all, err := e.store.For(project).AllTasks()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, d := range api.Descendants(all, id) {
		if !api.Open(d) || (scope == api.ScopeUnrated && d.Priority != "") {
			continue
		}
		out = append(out, d.ID)
	}
	return append(out, id), nil
}

// writePriority records one rating where that task's priority lives: its own row when sindri owns the
// task, the hub's overlay when the task is mirrored.
func (e *Engine) writePriority(project, id, priority string) error {
	ps := e.store.For(project)
	if ps.OwnsTask(id) {
		return ps.SetOwnedPriority(id, priority)
	}
	return ps.SetPriorityOverride(id, priority)
}
