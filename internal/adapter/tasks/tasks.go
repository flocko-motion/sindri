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
	// Name identifies the source for storage that must distinguish comment threads by origin —
	// not user-facing, so it owes nothing to display conventions.
	Name() string
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
	// Comments fetches taskID's comment thread. ok=false means this source keeps no thread of its
	// own for this id — not merely "no comments yet" — so the caller leaves what it already has
	// alone rather than reconcile against an empty result.
	Comments(root, taskID string) (cs []task.Comment, ok bool, err error)
	// AddComment posts to taskID's thread where the source keeps one of its own. handled=false
	// means this source has no such thread, so the caller records the comment itself instead.
	AddComment(root, taskID, body string) (handled bool, err error)
	// ToolMissing reports whether the repo's content calls for this source (independent of whether
	// its external tool happens to be installed) but that tool is not on PATH — the signal behind
	// "you'll want this, but it isn't set up." A source with no such split folds the check into
	// Enabled instead and always answers false here.
	ToolMissing(root string) bool
}
