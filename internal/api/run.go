// package: api / run
// type:    logic (wire types + a pure predicate)
// job:     a Run (a scheduled command in the run queue) as it crosses the wire, its
// detail view, and the open-ness rule the section badge and the filter share.
// limits:  data and a pure predicate only; no persistence, no scheduling.
package api

// Run is one scheduled command: an agent's ask, the hub's queue slot for it, and how it went.
type Run struct {
	Project string `json:"project"`
	ID      string `json:"id"`
	Agent   string `json:"agent"`   // who scheduled it
	Command string `json:"command"` // the shell command run in the agent's workspace
	// Status: queued | running | passed | failed | timed_out | cancelled.
	Status string `json:"status"`
	// Priority is a P-code (P0…P4), the same vocabulary tasks use — the queue runs its highest
	// first, creation order breaking ties. "" sorts last.
	Priority string `json:"priority,omitempty"`
	// Position is this run's place among currently queued runs, 1 = next; 0 once it is no longer
	// queued. Derived by the hub at read time, never stored — the queue's real order is a live
	// fact, not a column that could disagree with it.
	Position   int    `json:"position,omitempty"`
	CreatedAt  string `json:"created_at"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
	// UpdatedAt is stamped on every write, so the active filter can tell a run that just
	// finished from one that has sat done for a while.
	UpdatedAt string `json:"updated_at,omitempty"`
}

// RunDetail is a run plus its stored console output (for `run info` / the detail pane), kept off
// the list the way a PR's diff is: capped at render, fetched only when asked for.
type RunDetail struct {
	Run    Run    `json:"run"`
	Output string `json:"output"`
}

// RunOpen reports whether a run is still in play — queued or running, neither terminal state.
func RunOpen(r Run) bool { return r.Status == "queued" || r.Status == "running" }
