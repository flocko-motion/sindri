// package: adapter/tasks / tasks
// type:    logic (the task-source PORT)
// job:     the generic interface a task source implements — is it usable for a repo,
//          and fetch its tasks normalized to the domain Task entity (owning its id
//          scheme). td, spec, github each implement it; the hub iterates sources and
//          layers its own policy (GitHub opt-in + TTL, priority overrides) on top.
// limits:  no hub policy here — a Source just fetches + normalizes.
package tasks

import "github.com/flo-at/sindri/internal/hub/task"

// Source is a place tasks come from (td, openspec, GitHub). Each adapter implements
// it, mapping its own world onto task.Task — including the id scheme that namespaces
// the source (td-*, os-*, gh-*).
type Source interface {
	// Enabled reports whether this source is usable for the repo at root (e.g. the
	// repo uses openspec, or has a GitHub remote + gh). A disabled source is skipped.
	Enabled(root string) bool
	// Tasks fetches the source's tasks for the repo, normalized to task.Task.
	Tasks(root string) ([]task.Task, error)
}
