// package: api / prfilter
// type:    logic (a rule over the exchange types)
// job:     the four segments of the PR board a view can show, mirroring
// taskfilter.go so "active" means the same thing on both tabs.
// limits:  a pure predicate over PR; which one a view starts on is the view's own.
package api

import (
	"fmt"
	"strings"
	"time"
)

// PRFilter names a segment of the PR board.
type PRFilter string

// The segments — same spelling and meaning as TaskFilter.
const (
	PRFilterOpen   PRFilter = "open"   // PROpen: not merged, not scrapped
	PRFilterClosed PRFilter = "closed" // merged or scrapped
	PRFilterAll    PRFilter = "all"
	PRFilterActive PRFilter = "active" // open, plus whatever has just stopped being open
)

// PRFilters is the shared cycle order: the CLI lists it in its help, the TUI cycles through it.
var PRFilters = []PRFilter{PRFilterOpen, PRFilterClosed, PRFilterAll, PRFilterActive}

// MatchesPRFilter reports whether p belongs in the view f names. Active reuses ActiveWindow
// (-> taskfilter.go), so the word means the same thing one tab apart.
func MatchesPRFilter(f PRFilter, p PR) bool {
	switch f {
	case PRFilterOpen:
		return PROpen(p)
	case PRFilterClosed:
		return !PROpen(p)
	case PRFilterActive:
		return PROpen(p) || PRChangedWithin(p, ActiveWindow)
	}
	return true
}

// FilterPRs keeps the PRs f admits, in the order given.
func FilterPRs(f PRFilter, prs []PR) []PR {
	out := make([]PR, 0, len(prs))
	for _, p := range prs {
		if MatchesPRFilter(f, p) {
			out = append(out, p)
		}
	}
	return out
}

// PRChangedWithin reports whether p's last known change falls inside d.
func PRChangedWithin(p PR, d time.Duration) bool { return changedWithin(p.UpdatedAt, d) }

// NextPRFilter is the one after f, wrapping.
func NextPRFilter(f PRFilter) PRFilter {
	for i, c := range PRFilters {
		if c == f {
			return PRFilters[(i+1)%len(PRFilters)]
		}
	}
	return PRFilters[0]
}

// ParsePRFilter reads a filter by name, naming the whole set when it is not one of them.
func ParsePRFilter(s string) (PRFilter, error) {
	for _, c := range PRFilters {
		if s == string(c) {
			return c, nil
		}
	}
	return "", fmt.Errorf("unknown filter %q — one of: %s", s, PRFilterNames())
}

// PRFilterNames lists the filters for help text and errors, in the shared order.
func PRFilterNames() string {
	names := make([]string, len(PRFilters))
	for i, c := range PRFilters {
		names[i] = string(c)
	}
	return strings.Join(names, "|")
}
