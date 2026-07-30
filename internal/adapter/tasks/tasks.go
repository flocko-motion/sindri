// package: adapter/tasks / tasks
// type:    logic (the task-source PORT)
// job:     the generic interface a task source implements — is it usable for a repo,
// and fetch its tasks normalized to the domain Task entity (owning its id
// scheme). td, spec, github each implement it; the hub iterates sources and
// layers its own policy (GitHub opt-in + TTL, priority overrides) on top.
// limits:  no hub policy here — a Source just fetches + normalizes.
package tasks

import "github.com/flo-at/sindri/internal/hub/task"

// Source is a place tasks come from (td, openspec, GitHub), mapping its own world onto task.Task
// and namespacing its ids (td-*, os-*, gh-*). The hub never branches on which one is underneath.
type Source interface {
	// Enabled is the source's OWN gate for this repo; a disabled source is skipped.
	Enabled(root string) bool
	// Tasks fetches this source's tasks; force bypasses any internal cache. A network source
	// degrades to its last good result rather than failing the whole sync.
	Tasks(root string, force bool) ([]task.Task, error)
	// OnMerged is the source's consequence when a task's PR merges locally. Callers notify every
	// source blindly, so each acts only on its own ids; the error is for logging, never fatal.
	OnMerged(root, taskID, note string) error
	// Finish ends a task from the task list: scrap=false is "done", true is "discard". handled
	// reports whether THIS source owned the id, so an unknown backend can be flagged.
	Finish(root, taskID string, scrap bool) (handled bool, err error)
}
