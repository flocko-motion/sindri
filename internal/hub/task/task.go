// package: hub/task
// type:    logic (the Task entity)
// job:     sindri's Task domain entity — the generalization over its underlying
//          source types (td tasks, GitHub issues, openspec changes) into one unified
//          task the hub reasons about, plus the read filter. The domain owns the
//          entity; the task-source adapters translate their world to/from it.
// limits:  imports nothing internal; doesn't fetch (-> adapter/tasks) or render.
package task

import "time"

// Task is a unified sindri task — a td task, GitHub issue, or openspec change,
// normalized to one shape the hub works with regardless of source.
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
