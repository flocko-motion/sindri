// package: hub/task / view
// type:    logic (task presentation + ordering)
// job:     the shared, UI-agnostic view helpers over cached tasks: the priority/state
//          label mappings (CLI and TUI agree via these), the open/done predicate, and
//          arranging a flat task set into its parent/child tree (roots by priority
//          then id, each followed by its descendants, PR-annotated).
// limits:  pure functions over store rows; no rendering, no I/O. Board-badge counts
//          that need the whole BoardState live in the hub (sections.go).
package task

import (
	"sort"

	"github.com/flo-at/sindri/internal/hub/store"
)

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
// the open/closed filter.
func Done(t store.Task) bool {
	switch t.Status {
	case "closed", "approved", "merged":
		return true
	}
	return false
}

// Open reports whether a task still counts as open (not done).
func Open(t store.Task) bool { return !Done(t) }

// TaskRow is a task placed in the hierarchy: its tree depth, whether it is the last
// child of its parent (for drawing tree connectors), and the id of a non-merged PR
// for it (or "").
type TaskRow struct {
	store.Task
	Depth  int    `json:"depth"`
	Last   bool   `json:"last"`
	PR     string `json:"pr"`
	PRKind string `json:"pr_kind"` // "final" | "interim" — how to mark the PR on the row
}

// ArrangeTasks orders a flat task set into its parent/child tree — roots first (by
// priority then id), each immediately followed by its descendants, depth tagged — and
// annotates each row with a non-merged PR id if one exists. A task whose parent is
// absent from the set is treated as a root so nothing is hidden.
func ArrangeTasks(tasks []store.Task, prs []store.PR) []TaskRow {
	byParent := map[string][]store.Task{}
	present := map[string]bool{}
	for _, t := range tasks {
		present[t.ID] = true
	}
	for _, t := range tasks {
		p := t.ParentID
		if p == "" || !present[p] {
			p = "" // root (no parent, or parent not in the set)
		}
		byParent[p] = append(byParent[p], t)
	}
	for p := range byParent {
		sortTasks(byParent[p])
	}
	pr := map[string]store.PR{} // task id -> its open (non-terminal) PR
	for _, p := range prs {
		if p.Status != "merged" && p.Status != "scrapped" {
			pr[p.Task] = p
		}
	}

	var out []TaskRow
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		kids := byParent[parent]
		for i, t := range kids {
			out = append(out, TaskRow{Task: t, Depth: depth, Last: i == len(kids)-1, PR: pr[t.ID].ID, PRKind: pr[t.ID].Kind})
			walk(t.ID, depth+1)
		}
	}
	walk("", 0)
	return out
}

// sortTasks orders siblings: highest priority first (P0…P4, unset last), then id.
func sortTasks(ts []store.Task) {
	sort.SliceStable(ts, func(i, j int) bool {
		pi, pj := ts[i].Priority, ts[j].Priority
		if (pi == "") != (pj == "") {
			return pj == "" // non-empty before empty
		}
		if pi != pj {
			return pi < pj
		}
		return ts[i].ID < ts[j].ID
	})
}
