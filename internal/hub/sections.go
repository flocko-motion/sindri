// package: hub / sections
// type:    logic (view derivation the UIs render)
// job:     the single source of truth for the dashboard's sections (their badge
//          counts) and for arranging tasks into their parent/child tree with a
//          waiting-PR annotation. UIs render these; they never re-derive counts
//          or hierarchy.
// limits:  pure functions over BoardState/store types; no rendering, no I/O.
package hub

import (
	"sort"

	"github.com/flo-at/sindri/internal/hub/store"
)

// Section is one dashboard tab: a key, a title, and its actionable badge count
// derived from board state.
type Section struct {
	Key   string
	Title string
	Count func(BoardState) int
}

// Sections is the ordered set of dashboard sections — add one here and every UI
// picks it up.
var Sections = []Section{
	{"tasks", "Tasks", func(b BoardState) int { return countTasks(b.Tasks, taskOpen) }},
	{"agents", "Agents", func(b BoardState) int { return countAgents(b.Agents, agentRunning) }},
	{"prs", "PRs", func(b BoardState) int { return countPRs(b.PRs, prNotMerged) }},
}

// --- badge predicates ---

func taskOpen(t store.Task) bool   { return !taskDone(t) }
func agentRunning(a AgentView) bool { return a.Running }
func prNotMerged(p store.PR) bool  { return p.Status != "merged" }

// taskDone reports whether a task is in a terminal (done) state — the "closed"
// segment of the open/closed filter.
func taskDone(t store.Task) bool {
	switch t.Status {
	case "closed", "approved", "merged":
		return true
	}
	return false
}

func countTasks(ts []store.Task, pred func(store.Task) bool) (n int) {
	for _, t := range ts {
		if pred(t) {
			n++
		}
	}
	return
}
func countAgents(as []AgentView, pred func(AgentView) bool) (n int) {
	for _, a := range as {
		if pred(a) {
			n++
		}
	}
	return
}
func countPRs(ps []store.PR, pred func(store.PR) bool) (n int) {
	for _, p := range ps {
		if pred(p) {
			n++
		}
	}
	return
}

// --- task tree ---

// TaskRow is a task placed in the hierarchy: its tree depth, and the id of a
// non-merged PR for it (or "").
type TaskRow struct {
	store.Task
	Depth int    `json:"depth"`
	PR    string `json:"pr"`
}

// ArrangeTasks orders a flat task set into its parent/child tree — roots first
// (by priority then id), each immediately followed by its descendants, depth
// tagged — and annotates each row with a non-merged PR id if one exists. A task
// whose parent is absent from the set is treated as a root so nothing is hidden.
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
	pr := map[string]string{} // task id -> non-merged PR id
	for _, p := range prs {
		if p.Status != "merged" {
			pr[p.Task] = p.ID
		}
	}

	var out []TaskRow
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		for _, t := range byParent[parent] {
			out = append(out, TaskRow{Task: t, Depth: depth, PR: pr[t.ID]})
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
