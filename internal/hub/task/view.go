// package: hub/task / view
// type:    logic (task presentation)
// job:     the shared, UI-agnostic priority/state label mappings (CLI and TUI agree
// via these) and the pending-approval query. Arranging a flat task set into its
// tree, its descendants, and the open/done predicate cross the wire, so they live
// in internal/api; this package keeps its own names as aliases to them.
// limits:  pure functions over store rows; no rendering, no I/O. Board-badge counts
// that need the whole BoardState live in the hub (sections.go).
package task

import "github.com/flo-at/sindri/internal/api"

// PriorityLabel maps td's P0…P4 priority codes to readable words for display (sorting
// still uses the codes). Shared by the CLI and the TUI so they agree.
func PriorityLabel(p string) string {
	switch p {
	case "P0":
		return "critical"
	case "P1":
		return "high"
	case "P2":
		return "mid"
	case "P3":
		return "low"
	case "P4":
		return "none" // "came in unrated" — GitHub issues import here by default
	case "":
		return "-"
	default:
		return p
	}
}

// PriorityCode maps a readable word to td's P-code (the inverse of PriorityLabel). A
// value already in P-code form passes through.
func PriorityCode(word string) string {
	switch word {
	case "critical":
		return "P0"
	case "high":
		return "P1"
	case "mid", "medium":
		return "P2"
	case "low":
		return "P3"
	case "none", "trivial", "minor": // trivial/minor kept as back-compat input aliases
		return "P4"
	default:
		return word
	}
}

// PriorityWords are the assignable priorities, highest first (for choice menus).
var PriorityWords = []string{"critical", "high", "mid", "low", "none"}

// StateLabel maps a task status to a short, fixed-ish word for compact display (so the
// column doesn't need room for "in_progress"). Shared by CLI and TUI.
func StateLabel(s string) string {
	switch s {
	case "in_progress":
		return "active"
	case "in_review":
		return "review"
	case "closed":
		return "done"
	case "approved":
		return "appr"
	default:
		return s // open, merged, …
	}
}

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
// A rejected one is a verdict already given, and one that has ended decides nothing — so a
// cascading approve reaches neither, and both keep the state a human put them in.
func PendingApproval(tasks []api.Task, id string) []api.Task {
	var out []api.Task
	for _, d := range Descendants(tasks, id) {
		if Open(d) && d.Approval == "pending" {
			out = append(out, d)
		}
	}
	return out
}

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
