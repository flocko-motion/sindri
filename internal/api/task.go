// package: api / task
// type:    logic (wire types + pure functions)
// job:     the task as it crosses the wire (Task), placed in its hierarchy (TaskRow),
// and the spec a create/edit carries (TaskSpec) — plus the pure functions over
// them: arranging a flat set into a tree (ArrangeTasks), everything under an
// id (Descendants), and the open/done predicate every filter uses.
// limits:  data and pure functions only; no persistence, no rendering.
package api

import "sort"

// Task is the cached read-model row; large fields land only on a detail read.
type Task struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	// Tier is the task's difficulty estimate — junior/mid/senior, "" unrated (-> TierOrDefault, which
	// reads that as mid). A planner's guess at which worker model it is worth, not a claimability gate
	// the way Priority is.
	Tier        string `json:"tier,omitempty"`
	Type        string `json:"type"`
	Labels      string `json:"labels"` // comma-joined
	ParentID    string `json:"parent_id"`
	Description string `json:"description,omitempty"`
	Acceptance  string `json:"acceptance,omitempty"`
	URL         string `json:"url,omitempty"`        // an external permalink (e.g. a GitHub issue); "" if none
	UpdatedAt   string `json:"updated_at,omitempty"` // last status/field change at the source; "" if unknown
	// CreatedAt is when the task came into being at its source (RFC3339); "" if the source has no
	// answer. Never rewritten by a sync — the cached row is replaced wholesale each time, so a value
	// taken from "now" there would make every task look minutes old for ever.
	CreatedAt string `json:"created_at,omitempty"`
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
	PR     string `json:"pr"`
	PRKind string `json:"pr_kind"` // "final" | "interim" — how to mark the PR on the row
}

// TaskSpec is the full editable shape of a task — the payload of both create and
// edit. Empty fields mean "unset" (create) or "leave unchanged" (edit).
type TaskSpec struct {
	Title       string
	Type        string
	Priority    string // a P-code (P0…P4)
	Tier        string // junior|mid|senior, "" = unset (create) or unchanged (edit)
	Parent      string // parent task id (a child of this task)
	Description string
	Labels      []string
}

// Done reports whether a task is in a terminal (done) state — the "closed" segment of
// the open/closed filter.
func Done(t Task) bool { return DoneStatus(t.Status) }

// DoneStatus is Done for a caller holding a status word rather than the task — a row being
// coloured, a stored status being read back. The terminal words are listed once, here.
func DoneStatus(status string) bool {
	switch status {
	case "closed", "approved", "merged":
		return true
	}
	return false
}

// Open reports whether a task still counts as open (not done).
func Open(t Task) bool { return !Done(t) }

// ReleasedByPriority maps each task to whether a priority has released it for assignment — its own,
// or one on an ancestor. Rating an epic releases its whole tree (children are deliberately left
// unrated), so a bare "has no priority" would condemn nearly every subtask; only a task with no
// rated ancestor anywhere above it is actually unassignable.
func ReleasedByPriority(tasks []Task) map[string]bool {
	byID := make(map[string]Task, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t
	}
	out := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		seen := map[string]bool{}
		for cur, ok := t, true; ok && !seen[cur.ID]; cur, ok = byID[cur.ParentID] {
			seen[cur.ID] = true
			if cur.Priority != "" {
				out[t.ID] = true
				break
			}
		}
	}
	return out
}

// AwaitingVerdict reports an open task the user has not ruled on. Such a task is hidden from every
// worker, so a backlog full of them looks busy while nothing can be claimed — which is why the
// count is surfaced rather than left for someone to work out from the rows.
func AwaitingVerdict(t Task) bool { return Open(t) && t.Approval == "pending" }

// TaskNeedsUser reports a task stopped behind a gate only the user can open — awaiting a verdict,
// or unreleased by any priority (released is ReleasedByPriority's answer, which reads the tree).
// Both gates must pass for a worker to be handed it. What the badge counts and what a red row says.
// A REJECTED task is out whatever its rating: the user has ruled, rating releases nothing while the
// rejection stands, and the next move belongs to whoever revises it.
func TaskNeedsUser(t Task, released bool) bool {
	if !Open(t) || t.Approval == "rejected" {
		return false
	}
	return t.Approval == "pending" || !released
}

// CountTasksNeedingUser is how many of these tasks are stopped behind one of those gates.
func CountTasksNeedingUser(tasks []Task) (n int) {
	released := ReleasedByPriority(tasks)
	for _, t := range tasks {
		if TaskNeedsUser(t, released[t.ID]) {
			n++
		}
	}
	return n
}

// CountAwaitingVerdict is how many of these tasks are waiting on the user.
func CountAwaitingVerdict(tasks []Task) (n int) {
	for _, t := range tasks {
		if AwaitingVerdict(t) {
			n++
		}
	}
	return n
}

// PendingApproval returns the tasks under id still waiting on the user's verdict, deepest first.
// A rejected one is a verdict already given, and one that has ended decides nothing — so a
// cascading approve reaches neither, and both keep the state a human put them in.
func PendingApproval(tasks []Task, id string) []Task {
	var out []Task
	for _, d := range Descendants(tasks, id) {
		if Open(d) && d.Approval == "pending" {
			out = append(out, d)
		}
	}
	return out
}

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
		for _, t := range byParent[parent] {
			out = append(out, TaskRow{Task: t, Depth: depth, PR: pr[t.ID].ID, PRKind: pr[t.ID].Kind})
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
