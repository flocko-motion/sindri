// Package issue is the headless domain model for sindri work items.
//
// A work item (Issue) is the union of an optional td task and an optional
// openspec change. It models three shapes:
//   - a task with no spec        (Task != nil, Spec == nil)
//   - a task implementing a spec (Task != nil, Spec != nil)
//   - a spec with no task yet    (Task == nil, Spec != nil)
//
// All label and state logic lives here — review gates, spec links, status
// classification — with no dependency on any UI. Interfaces consume Issue and
// render it; they do not reimplement this logic.
package issue

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
)

const (
	// Review-gate labels are require-review-<type> / approved-review-<type>.
	// We strip only "require-"/"approved-", keeping "review-<type>" as the gate
	// name so it displays as "review <type>" (e.g. "review code").
	requirePrefix  = "require-"
	approvedPrefix = "approved-"
	requireMatch   = "require-review-"
	approvedMatch  = "approved-review-"
	specPrefix     = "spec:"
)

// Task is a td task.
type Task struct {
	ID        string
	Title     string
	Status    string
	Type      string
	Priority  string
	Labels    []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Spec is an openspec change (proposal). TotalTasks/CompletedTasks come from
// its tasks.md checklist when available.
type Spec struct {
	Name           string
	CompletedTasks int
	TotalTasks     int
}

// Gate is a single review gate and whether it has been approved.
type Gate struct {
	Name     string // e.g. "review-code", "review-security"
	Approved bool
}

// PR is an associated pull request (display projection of store.PR).
type PR struct {
	ID     string
	Status string
	Branch string
	Base   string
	Title  string
}

// --- Task state & label logic ---

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

// IsOpen reports whether the task is open (unclaimed).
func (t Task) IsOpen() bool { return t.Status == "open" }

// SpecName returns the linked openspec change name (spec:<name> label), or "".
func (t Task) SpecName() string {
	for _, l := range t.Labels {
		if strings.HasPrefix(l, specPrefix) {
			return strings.TrimPrefix(l, specPrefix)
		}
	}
	return ""
}

// Gates returns the review gates declared on the task, with approval state.
func (t Task) Gates() []Gate {
	approved := map[string]bool{}
	var names []string
	for _, l := range t.Labels {
		if strings.HasPrefix(l, approvedMatch) {
			approved[strings.TrimPrefix(l, approvedPrefix)] = true
		}
	}
	for _, l := range t.Labels {
		if strings.HasPrefix(l, requireMatch) {
			names = append(names, strings.TrimPrefix(l, requirePrefix))
		}
	}
	gates := make([]Gate, 0, len(names))
	for _, n := range names {
		gates = append(gates, Gate{Name: n, Approved: approved[n]})
	}
	return gates
}

// MissingReviews returns the names of required reviews not yet approved.
func (t Task) MissingReviews() []string {
	var missing []string
	for _, g := range t.Gates() {
		if !g.Approved {
			missing = append(missing, g.Name)
		}
	}
	return missing
}

// --- Issue: the task/spec union ---

// Issue is the unified view-model the UIs render: a task and/or a spec, plus
// the assigned worker and associated PRs. At least one of Task or Spec is set.
type Issue struct {
	Task   *Task
	Spec   *Spec
	Worker string // assigned worker name, "" if none
	PRs    []PR
}

// HasTask reports whether the issue has a td task.
func (i Issue) HasTask() bool { return i.Task != nil }

// HasSpec reports whether the issue is linked to (or is) an openspec change.
func (i Issue) HasSpec() bool { return i.Spec != nil }

// SpecOnly reports whether this is a spec with no task yet.
func (i Issue) SpecOnly() bool { return i.Task == nil && i.Spec != nil }

// ID is the task ID (td-xxxxxx), or a synthetic os-xxxxxx id derived from the
// spec name for a spec-only issue (so every row has a td-/os- style ID).
func (i Issue) ID() string {
	if i.Task != nil {
		return i.Task.ID
	}
	if i.Spec != nil {
		return SpecID(i.Spec.Name)
	}
	return ""
}

// SpecID derives a stable "os-xxxxxx" id from an openspec change name.
func SpecID(name string) string {
	h := sha256.Sum256([]byte(name))
	return "os-" + hex.EncodeToString(h[:])[:6]
}

// Title is the task title, with a spec marker prefix when linked. For a
// spec-only issue it explains the missing task.
func (i Issue) Title() string {
	if i.Task == nil {
		return "(no task — needs planning)"
	}
	if i.Spec != nil {
		return "📋 " + i.Spec.Name + " · " + i.Task.Title
	}
	return i.Task.Title
}

// Priority returns the task priority, or "" for a spec-only issue.
func (i Issue) Priority() string {
	if i.Task != nil {
		return i.Task.Priority
	}
	return ""
}

// UpdatedAt returns the task's last-updated time (zero for spec-only).
func (i Issue) UpdatedAt() time.Time {
	if i.Task != nil {
		return i.Task.UpdatedAt
	}
	return time.Time{}
}

// IsClosed reports whether the issue's task is terminal. Spec-only is never closed.
func (i Issue) IsClosed() bool { return i.Task != nil && i.Task.IsClosed() }

// Gates returns the task's review gates (nil for spec-only).
func (i Issue) Gates() []Gate {
	if i.Task != nil {
		return i.Task.Gates()
	}
	return nil
}

// SpecProgress returns the spec's checklist progress and whether it applies.
func (i Issue) SpecProgress() (done, total int, ok bool) {
	if i.Spec == nil {
		return 0, 0, false
	}
	return i.Spec.CompletedTasks, i.Spec.TotalTasks, true
}

// Status returns the raw status token for the issue: the task status, or
// "spec" for a spec-only item. Coloring is the renderer's job.
func (i Issue) Status() string {
	if i.Task != nil {
		return i.Task.Status
	}
	return "spec"
}

// Assemble pairs tasks and specs into unified issues and attaches each task's
// worker and PRs. Each task links to at most one spec (its spec:<name> label);
// each spec is owned by at most one task. The result lists specs that no task
// claims first (spec-only, needing planning), then the tasks in their given
// order. workerByTask and prsByTask are keyed by task ID and may be nil.
//
// Assemble is pure: callers gather the inputs (the data layer fetches td,
// openspec, workers, and PRs); issue itself imports nothing.
func Assemble(tasks []Task, specs []Spec, workerByTask map[string]string, prsByTask map[string][]PR) []Issue {
	specByName := map[string]*Spec{}
	for i := range specs {
		specByName[specs[i].Name] = &specs[i]
	}
	claimed := map[string]bool{}

	var withTask []Issue
	for i := range tasks {
		iss := Issue{Task: &tasks[i], Worker: workerByTask[tasks[i].ID], PRs: prsByTask[tasks[i].ID]}
		if sn := tasks[i].SpecName(); sn != "" {
			iss.Spec = specByName[sn]
			claimed[sn] = true
		}
		withTask = append(withTask, iss)
	}

	var issues []Issue
	for i := range specs {
		if !claimed[specs[i].Name] {
			issues = append(issues, Issue{Spec: &specs[i]})
		}
	}
	return append(issues, withTask...)
}

var taskIDRe = regexp.MustCompile(`\(?(td-[0-9a-f]+)\)?`)

// TaskIDFromTitle extracts a td-xxxxxx task ID from a PR title, or "".
func TaskIDFromTitle(title string) string {
	if m := taskIDRe.FindStringSubmatch(title); len(m) > 1 {
		return m[1]
	}
	return ""
}

