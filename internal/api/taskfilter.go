// package: api / taskfilter
// type:    logic (a rule over the exchange types)
// job:     the four segments of the backlog a view can show, what each admits, and how
// recent "active" means — one definition the CLI flag and the TUI cycle share.
// limits:  a pure predicate over Task; which one a view starts on is the view's own.
package api

import (
	"fmt"
	"strings"
	"time"
)

// TaskFilter names a segment of the backlog.
type TaskFilter string

// The segments. Their spelling is the CLI's flag value and the word the TUI footer shows.
const (
	FilterOpen   TaskFilter = "open"   // not done: open, in_progress, in_review
	FilterClosed TaskFilter = "closed" // the done segment: closed, approved, merged
	FilterAll    TaskFilter = "all"    // every task, whatever its status
	FilterActive TaskFilter = "active" // open work, plus whatever has just stopped being open
)

// TaskFilters is the order both front-ends present: the CLI lists it in its help, the TUI cycles
// through it, so the two describe the same set the same way round.
var TaskFilters = []TaskFilter{FilterOpen, FilterClosed, FilterAll, FilterActive}

// ActiveWindow is how recently a done task must have changed to still count as active — wide
// enough that work finished just before you looked has not already vanished.
const ActiveWindow = 2 * time.Hour

// MatchesFilter reports whether t belongs in the view f names. Active is a UNION — everything not
// done, PLUS anything whose status changed inside ActiveWindow — so a task closed a moment ago
// stays on screen and the work just finished leaves a trace.
//
// A task carrying no timestamp is never recent, so it shows under active only by being open. Every
// source is expected to date its tasks; where one cannot, its closed tasks are reachable under
// "closed" and "all" rather than silently absent everywhere.
//
// An unrecognised filter admits everything: a listing that showed nothing would read as an empty
// backlog, which is a lie a wrong flag should not be able to tell.
func MatchesFilter(f TaskFilter, t Task) bool {
	switch f {
	case FilterOpen:
		return Open(t)
	case FilterClosed:
		return Done(t)
	case FilterActive:
		return Open(t) || ChangedWithin(t, ActiveWindow)
	}
	return true
}

// FilterTasks keeps the tasks the filter admits, in the order given.
func FilterTasks(f TaskFilter, tasks []Task) []Task {
	out := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		if MatchesFilter(f, t) {
			out = append(out, t)
		}
	}
	return out
}

// TaskListSummary is the line a filtered listing closes with, in both front-ends and in the agent's
// own verb. A narrow default is only safe alongside it: it says what was shown AND the shape of the
// whole backlog, so a listing of fourteen rows can never read as a backlog of fourteen tasks.
func TaskListSummary(f TaskFilter, shown int, tasks []Task) string {
	open, closed := 0, 0
	for _, t := range tasks {
		if Done(t) {
			closed++
		} else {
			open++
		}
	}
	head := fmt.Sprintf("showing %d %s %s", shown, f, plural(shown, "task"))
	if f == FilterAll {
		head = fmt.Sprintf("showing all %d %s", shown, plural(shown, "task"))
	}
	return fmt.Sprintf("%s — %d open, %d closed in total", head, open, closed)
}

// plural is the noun for n of something, for a count a human reads.
func plural(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

// WithAncestors adds every ancestor of a kept task back to the set, and reports which ids were added
// for that reason alone. A listing indented by tree needs them: a subtask whose parent the filter
// dropped would otherwise sit at a depth that means nothing, or be re-rooted as though it belonged to
// nobody — and a planner reads this view to see exactly which tree a task hangs in.
func WithAncestors(kept, all []Task) ([]Task, map[string]bool) {
	byID := make(map[string]Task, len(all))
	for _, t := range all {
		byID[t.ID] = t
	}
	in := make(map[string]bool, len(kept))
	for _, t := range kept {
		in[t.ID] = true
	}
	context := map[string]bool{}
	for _, t := range kept {
		// Stops on a stored loop rather than spinning: a parent chain is data, and data can be wrong.
		for at := t.ParentID; at != "" && !in[at] && !context[at]; at = byID[at].ParentID {
			if _, ok := byID[at]; !ok {
				break // a parent outside this set entirely; ArrangeTasks re-roots the child
			}
			context[at] = true
		}
	}
	if len(context) == 0 {
		return kept, nil
	}
	// Rebuilt in the order `all` gives, so the result keeps whatever order the caller sorted by.
	out := make([]Task, 0, len(kept)+len(context))
	for _, t := range all {
		if in[t.ID] || context[t.ID] {
			out = append(out, t)
		}
	}
	return out, context
}

// ChangedWithin reports whether a task's last known change falls inside d. An absent or unparsable
// timestamp is no evidence of recency, so it answers false.
func ChangedWithin(t Task, d time.Duration) bool { return changedWithin(t.UpdatedAt, d) }

// changedWithin is the recency check behind ChangedWithin and PRChangedWithin (-> prfilter.go):
// the same rule, shared rather than copied, since both filters' "active" means it.
func changedWithin(ts string, d time.Duration) bool {
	at, err := time.Parse(time.RFC3339, ts)
	return err == nil && time.Since(at) < d
}

// NextTaskFilter is the one after f, wrapping — the cycle a front-end walks. An unknown filter
// lands on the first, so a view can never get stuck outside the set.
func NextTaskFilter(f TaskFilter) TaskFilter {
	for i, c := range TaskFilters {
		if c == f {
			return TaskFilters[(i+1)%len(TaskFilters)]
		}
	}
	return TaskFilters[0]
}

// ParseTaskFilter reads a filter by name, naming the whole set when it is not one of them.
func ParseTaskFilter(s string) (TaskFilter, error) {
	for _, c := range TaskFilters {
		if s == string(c) {
			return c, nil
		}
	}
	return "", fmt.Errorf("unknown filter %q — one of: %s", s, TaskFilterNames())
}

// TaskFilterNames lists the filters for help text and errors, in the shared order.
func TaskFilterNames() string {
	names := make([]string, len(TaskFilters))
	for i, c := range TaskFilters {
		names[i] = string(c)
	}
	return strings.Join(names, "|")
}
