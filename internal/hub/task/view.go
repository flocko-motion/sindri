// package: hub/task / view
// type:    logic (task presentation)
// job:     aliases to internal/api's tree/descendants/open-done/pending-approval —
// they cross the wire, so that's where they live now; hub/workflow keeps
// calling them by these names.
// limits:  pure functions over store rows; no rendering, no I/O. Board-badge counts
// that need the whole BoardState live in the hub (sections.go).
package task

import "github.com/flo-at/sindri/internal/api"

// Done reports whether a task is in a terminal (done) state — the "closed" segment of
// the open/closed filter. It crosses the wire, so it lives in internal/api; this is
// that function, under the name every existing caller here already uses.
var Done = api.Done

// Open reports whether a task still counts as open (not done).
var Open = api.Open

// Descendants returns everything under id — children, grandchildren, … — deepest
// first, the order a cascading scrap deletes in (a looping parent chain walks once).
var Descendants = api.Descendants

// PendingApproval returns the tasks under id still waiting on the user's verdict, deepest first.
var PendingApproval = api.PendingApproval

// TaskRow is a task placed in the hierarchy: its tree depth, whether it is the last
// child of its parent (for drawing tree connectors), and the id of a non-merged PR
// for it (or ""). It crosses the wire, so it is internal/api.TaskRow under the name
// every existing caller here already uses.
type TaskRow = api.TaskRow

// ArrangeTasks orders a flat task set into its parent/child tree — roots first (by
// priority then id), each immediately followed by its descendants, depth tagged — and
// annotates each row with a non-merged PR id if one exists. A task whose parent is
// absent from the set is treated as a root so nothing is hidden.
var ArrangeTasks = api.ArrangeTasks
