// package: issue
// type:    logic (bottom primitive)
// job:     the task domain model the td adapter produces and the hub consumes —
//          a td task plus the read filter. The old task/spec view-model union
//          (Issue/Assemble/gates) lived here too; it was the legacy board's and
//          was removed with that stack (hub owns the board now).
// limits:  imports nothing internal; doesn't fetch (-> adapter/td) or render.
package issue

import "time"

// Task is a td task.
type Task struct {
	ID        string
	Title     string
	Status    string
	Type      string
	Priority  string
	ParentID  string
	Labels    []string
	CreatedAt time.Time
	UpdatedAt time.Time
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
