// package: api / run
// type:    logic (wire types + a pure predicate)
// job:     a Run (a scheduled command in the run queue) as it crosses the wire, its
// detail view, and the open-ness rule the section badge and the filter share.
// limits:  data and a pure predicate only; no persistence, no scheduling.
package api

// Run is one scheduled command: the ask, the hub's queue slot for it, and how it went.
type Run struct {
	Project string `json:"project"`
	ID      string `json:"id"`
	// Agent is who scheduled it: an agent's name, or SenderUser for one the human queued. One
	// column, because "who asked" has one answer — and that sentinel is already the human's word.
	Agent   string `json:"agent"`
	Command string `json:"command"` // the shell command, run against Workspace
	// Status: queued | running | passed | failed | timed_out | cancelled.
	Status string `json:"status"`
	// Priority is a P-code (P0…P4), the same vocabulary tasks use — the queue runs its highest
	// first, creation order breaking ties. "" sorts last.
	Priority string `json:"priority,omitempty"`
	// Timeout is the agent-requested budget (a Go duration string, e.g. "5m"); "" defers to the
	// hub's hard cap. It never overrides that cap — only narrows it.
	Timeout string `json:"timeout,omitempty"`
	// Workspace is the repo-relative worktree this run executes against, fixed when it was
	// scheduled: an agent's, or "." for the repo's own checkout. Authoritative — the executor mounts
	// a COPY of it and never re-derives it, so a run tests the tree it was aimed at.
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
	// wait so the eventual commit reads the same as if the gate had run inline. A PR gate carries
	// the PR id here instead. Unused ("") for an ordinary run.
	Message string `json:"message,omitempty"`
	// Commit is the sha a gate run checks, recorded when it was queued: the verdict describes that
	// commit, not "whatever was on disk", which is what makes it reusable. "" for an ordinary run.
	Commit string `json:"commit,omitempty"`
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

// RunFromUser reports a run the human queued. Somebody is WATCHING it, so it leads the queue, and
// nothing treating Agent as an agent name — roster lookup, staleness, an injected result — applies.
// One predicate, so queue, executor and both front-ends cannot disagree about whose run it is.
func RunFromUser(r Run) bool { return r.Agent == SenderUser }

// RunTarget names the tree a run executes against: the same command passes in one workspace and
// fails in another, so a row without it is uninterpretable.
func RunTarget(r Run) string {
	if r.Workspace == "" || r.Workspace == "." {
		return "the repo's checkout"
	}
	return r.Workspace
}
