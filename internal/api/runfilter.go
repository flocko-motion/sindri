// package: api / runfilter
// type:    logic (a rule over the exchange types)
// job:     the four segments of the run queue a view can show, mirroring
// taskfilter.go and prfilter.go so "active" means the same thing everywhere.
// limits:  a pure predicate over Run; which one a view starts on is the view's own.
package api

import (
	"fmt"
	"strings"
	"time"
)

// RunFilter names a segment of the run queue.
type RunFilter string

// The segments — same spelling and meaning as TaskFilter/PRFilter.
const (
	RunFilterOpen   RunFilter = "open"   // RunOpen: queued or running
	RunFilterClosed RunFilter = "closed" // passed, failed, timed out, or cancelled
	RunFilterAll    RunFilter = "all"
	RunFilterActive RunFilter = "active" // open, plus whatever has just stopped being open
)

// RunFilters is the shared cycle order: the CLI lists it in its help, the TUI cycles through it.
var RunFilters = []RunFilter{RunFilterOpen, RunFilterClosed, RunFilterAll, RunFilterActive}

// MatchesRunFilter reports whether r belongs in the view f names. Active reuses ActiveWindow
// (-> taskfilter.go), so the word means the same thing on every tab.
func MatchesRunFilter(f RunFilter, r Run) bool {
	switch f {
	case RunFilterOpen:
		return RunOpen(r)
	case RunFilterClosed:
		return !RunOpen(r)
	case RunFilterActive:
		return RunOpen(r) || RunChangedWithin(r, ActiveWindow)
	}
	return true
}

// FilterRuns keeps the runs f admits, in the order given.
func FilterRuns(f RunFilter, runs []Run) []Run {
	out := make([]Run, 0, len(runs))
	for _, r := range runs {
		if MatchesRunFilter(f, r) {
			out = append(out, r)
		}
	}
	return out
}

// RunChangedWithin reports whether r's last known change falls inside d.
func RunChangedWithin(r Run, d time.Duration) bool { return changedWithin(r.UpdatedAt, d) }

// NextRunFilter is the one after f, wrapping.
func NextRunFilter(f RunFilter) RunFilter {
	for i, c := range RunFilters {
		if c == f {
			return RunFilters[(i+1)%len(RunFilters)]
		}
	}
	return RunFilters[0]
}

// ParseRunFilter reads a filter by name, naming the whole set when it is not one of them.
func ParseRunFilter(s string) (RunFilter, error) {
	for _, c := range RunFilters {
		if s == string(c) {
			return c, nil
		}
	}
	return "", fmt.Errorf("unknown filter %q — one of: %s", s, RunFilterNames())
}

// RunFilterNames lists the filters for help text and errors, in the shared order.
func RunFilterNames() string {
	names := make([]string, len(RunFilters))
	for i, c := range RunFilters {
		names[i] = string(c)
	}
	return strings.Join(names, "|")
}
