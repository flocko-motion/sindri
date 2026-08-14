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
	// Timeout is the agent-requested budget (a Go duration string, e.g. "5m"); "" defers to the
	// hub's hard cap. It never overrides that cap — only narrows it.
	Timeout string `json:"timeout,omitempty"`
	// Workspace is the repo-relative worktree path this run executes against (the agent's, at the
	// time it was scheduled).
	Workspace string `json:"workspace,omitempty"`
	// Task is the agent's task at schedule time, "" if it held none. A dequeue where the agent's
	// current task no longer matches means the agent moved on while this sat queued — the record
	// this run was meant to test may no longer exist in its workspace.
	Task string `json:"task,omitempty"`
	// ExitCode is the command's process exit code once it finishes: 0 on pass, the command's own
	// code on failure, -1 for a timeout or a cancellation (nothing to exit on its own).
	ExitCode int `json:"exit_code,omitempty"`
	// Kind is "" for an ordinary agent-requested run, or the submit-gate purpose ("submit" |
	// "contribute") that routes its result back through the submit/contribute flow instead of a
	// plain summary. Gate runs outrank ordinary ones in the queue by default.
	Kind string `json:"kind,omitempty"`
	// Message is the agent's free-text submit/contribute description, carried across the queue
	// wait so the eventual commit reads the same as if the gate had run inline. Unused ("") for an
	// ordinary run.
	Message string `json:"message,omitempty"`
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
