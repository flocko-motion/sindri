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
// the source (td-*, os-*, gh-*). The hub treats every source identically: it never
// knows or branches on which concrete source is underneath.
type Source interface {
	// Enabled reports whether this source is usable for the repo at root — the
	// source's OWN gate (repo uses openspec; a GitHub remote + gh + the project's
	// issues opt-in). A disabled source is skipped.
	Enabled(root string) bool
	// Tasks fetches the source's tasks for the repo, normalized to task.Task. force
	// asks for fresh data, bypassing any internal cache (a caching source honors it;
	// a cheap local source ignores it). A source that fetches over the network
	// degrades on error to its last good result rather than failing the whole sync.
	Tasks(root string, force bool) ([]task.Task, error)
}
