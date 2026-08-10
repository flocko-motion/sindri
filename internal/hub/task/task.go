// package: hub/task
// type:    logic (the Task entity)
// job:     sindri's Task domain entity — the generalization over its underlying
// source types (the hub's own tasks, GitHub issues, openspec changes) into one
// unified task the hub reasons about, plus the read filter. The domain owns
// the entity; each source translates its world to/from it.
// limits:  imports nothing internal; doesn't fetch (-> adapter/tasks) or render.
package task

import "time"

// Task is a unified sindri task — one the hub owns, a GitHub issue, or an openspec
// change, normalized to one shape the hub works with regardless of source.
type Task struct {
	ID          string
	Title       string
	Status      string
	Type        string
	Priority    string
	ParentID    string
	Labels      []string
	Description string // the body, when the source carries one up front (e.g. a GitHub issue)
	URL         string // an external permalink (e.g. the GitHub issue); "" when the source has none
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IsClosed reports whether the task is in a terminal state.
func (t Task) IsClosed() bool {
	switch t.Status {
	case "closed", "approved", "merged":
		return true
	}
	return false
}

// IsActive reports whether the task is being worked on or reviewed.
func (t Task) IsActive() bool {
	return t.Status == "in_progress" || t.Status == "in_review"
}

// Filter selects which tasks a read returns. It is UI-neutral.
type Filter int

const (
	FilterOpen   Filter = iota // hide closed tasks (the default)
	FilterAll                  // every task
	FilterClosed               // only closed tasks
)

// Comment is one comment on a task's thread, normalized the same way Task is: a source maps its own
// world onto this shape rather than the hub knowing the source's.
type Comment struct {
	SourceRef string // external id/url, unique within the source
	Author    string
	Body      string
	CreatedAt string // RFC3339
}
