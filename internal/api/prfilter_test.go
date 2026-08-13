package api

import (
	"strings"
	"testing"
	"time"
)

// board is one PR per case the filters have to separate.
func board() []PR {
	now := time.Now().UTC()
	return []PR{
		{ID: "open", Status: "open"},
		{ID: "approved", Status: "approved"},
		{ID: "rejected-undated", Status: "rejected"},
		{ID: "merged-now", Status: "merged", UpdatedAt: now.Format(time.RFC3339)},
		{ID: "merged-old", Status: "merged", UpdatedAt: now.Add(-3 * time.Hour).Format(time.RFC3339)},
		{ID: "scrapped-undated", Status: "scrapped"},
	}
}

func prIDs(prs []PR) string {
	out := make([]string, len(prs))
	for i, p := range prs {
		out[i] = p.ID
	}
	return strings.Join(out, ",")
}

// TestEachPRFilterAdmitsItsOwnSegment mirrors TestEachFilterAdmitsItsOwnSegment: active is the
// union of everything open with everything just closed, so a PR merged moments ago is still on
// screen and one merged hours ago is not.
func TestEachPRFilterAdmitsItsOwnSegment(t *testing.T) {
	want := map[PRFilter]string{
		PRFilterOpen:   "open,approved,rejected-undated",
		PRFilterClosed: "merged-now,merged-old,scrapped-undated",
		PRFilterAll:    "open,approved,rejected-undated,merged-now,merged-old,scrapped-undated",
		PRFilterActive: "open,approved,rejected-undated,merged-now",
	}
	for f, w := range want {
		if got := prIDs(FilterPRs(f, board())); got != w {
			t.Errorf("filter %q admitted %q, want %q", f, got, w)
		}
	}
}

// TestAnUndatedPRIsNeverRecent mirrors the task version: no timestamp is no evidence of recency.
func TestAnUndatedPRIsNeverRecent(t *testing.T) {
	undated := PR{ID: "x", Status: "scrapped"}
	if PRChangedWithin(undated, ActiveWindow) {
		t.Error("a PR with no timestamp cannot be known to be recent")
	}
	if MatchesPRFilter(PRFilterActive, undated) {
		t.Error("an undated scrapped PR must not show under active")
	}
	if !MatchesPRFilter(PRFilterClosed, undated) || !MatchesPRFilter(PRFilterAll, undated) {
		t.Error("it must still be reachable under closed and all")
	}
}

// TestParsePRFilterNamesTheWholeSet mirrors the task version.
func TestParsePRFilterNamesTheWholeSet(t *testing.T) {
	for _, f := range PRFilters {
		got, err := ParsePRFilter(string(f))
		if err != nil || got != f {
			t.Errorf("ParsePRFilter(%q) = %q, %v", f, got, err)
		}
	}
	_, err := ParsePRFilter("opne")
	if err == nil {
		t.Fatal("an unknown filter must be rejected, not silently accepted")
	}
	for _, f := range PRFilters {
		if !strings.Contains(err.Error(), string(f)) {
			t.Errorf("the error should name %q: %v", f, err)
		}
	}
}

// TestNextPRFilterCyclesAndWraps mirrors the task version.
func TestNextPRFilterCyclesAndWraps(t *testing.T) {
	f := PRFilters[0]
	for i := range PRFilters {
		next := NextPRFilter(f)
		if want := PRFilters[(i+1)%len(PRFilters)]; next != want {
			t.Fatalf("after %q the cycle gave %q, want %q", f, next, want)
		}
		f = next
	}
	if got := NextPRFilter("nonsense"); got != PRFilters[0] {
		t.Errorf("an unknown filter should rejoin at %q, got %q", PRFilters[0], got)
	}
}

// TestPRFilterSharesTheTaskWindow is the point of the whole task: "active" must mean the same
// span of time on both tabs, not a value that happens to match today.
func TestPRFilterSharesTheTaskWindow(t *testing.T) {
	if ActiveWindow != 2*time.Hour {
		t.Fatalf("ActiveWindow changed to %v — the PR filter reuses it directly, so this pins the shared value", ActiveWindow)
	}
}
