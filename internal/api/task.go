// package: api / task
// type:    data (wire types + pure functions)
// job:     the task as it crosses the wire (Task), placed in its hierarchy (TaskRow),
// and the spec a create/edit carries (TaskSpec) — plus the pure functions over
// them: arranging a flat set into a tree (ArrangeTasks), everything under an
// id (Descendants), and the open/done predicate every filter uses.
// limits:  data and pure functions only; no persistence, no rendering.
package api

import "sort"

// Task is the cached read-model row; large fields land only on a detail read.
type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	Type        string `json:"type"`
	Labels      string `json:"labels"` // comma-joined
	ParentID    string `json:"parent_id"`
	Description string `json:"description,omitempty"`
	Acceptance  string `json:"acceptance,omitempty"`
	URL         string `json:"url,omitempty"`        // an external permalink (e.g. a GitHub issue); "" if none
	UpdatedAt   string `json:"updated_at,omitempty"` // last status/field change at the source; "" if unknown
	// Approval gates planner-created tasks: "" (none), pending, approved, rejected.
	// Workers only ever see "" and approved tasks.
	Approval        string `json:"approval,omitempty"`
	ApprovalComment string `json:"approval_comment,omitempty"`
	// Comments is not a tasks column: TaskInfo assembles it, so it's empty elsewhere.
	Comments []Comment `json:"comments,omitempty"`
}

// TaskRow is a task placed in the hierarchy: its tree depth, whether it is the last
// child of its parent (for drawing tree connectors), and the id of a non-merged PR
// for it (or "").
type TaskRow struct {
	Task
	Depth  int    `json:"depth"`
	Last   bool   `json:"last"`
	PR     string `json:"pr"`
	PRKind string `json:"pr_kind"` // "final" | "interim" — how to mark the PR on the row
}

// TaskSpec is the full editable shape of a task — the payload of both create and
// edit. Empty fields mean "unset" (create) or "leave unchanged" (edit).
type TaskSpec struct {
	Title       string
	Type        string
	Priority    string // a P-code (P0…P4)
	Parent      string // parent task id (a child of this task)
	Description string
	Labels      []string
}

// Done reports whether a task is in a terminal (done) state — the "closed" segment of
// the open/closed filter.
func Done(t Task) bool {
	switch t.Status {
	case "closed", "approved", "merged":
		return true
	}
	return false
}

// Open reports whether a task still counts as open (not done).
func Open(t Task) bool { return !Done(t) }

// Descendants returns everything under id — children, grandchildren, … — deepest
// first, the order a cascading scrap deletes in (a looping parent chain walks once).
func Descendants(tasks []Task, id string) []Task {
	byParent := map[string][]Task{}
	for _, t := range tasks {
		if t.ParentID != "" && t.ID != t.ParentID {
			byParent[t.ParentID] = append(byParent[t.ParentID], t)
		}
	}
	for p := range byParent {
		sortTasks(byParent[p])
	}
	seen := map[string]bool{id: true}
	var out []Task
	var walk func(parent string)
	walk = func(parent string) {
		for _, t := range byParent[parent] {
			if seen[t.ID] {
				continue
			}
			seen[t.ID] = true
			walk(t.ID)
			out = append(out, t) // after its own subtree: deepest first
		}
	}
	walk(id)
	return out
}

// ArrangeTasks orders a flat task set into its parent/child tree — roots first (by
// priority then id), each immediately followed by its descendants, depth tagged — and
// annotates each row with a non-merged PR id if one exists. A task whose parent is
// absent from the set is treated as a root so nothing is hidden.
func ArrangeTasks(tasks []Task, prs []PR) []TaskRow {
	byParent := map[string][]Task{}
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
	pr := map[string]PR{} // task id -> its open (non-terminal) PR
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
func sortTasks(ts []Task) {
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
