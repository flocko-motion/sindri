package api

import (
	"strings"
	"testing"
	"time"
)

// queue is one run per case the filters have to separate.
func queue() []Run {
	now := time.Now().UTC()
	return []Run{
		{ID: "queued", Status: "queued"},
		{ID: "running", Status: "running"},
		{ID: "passed-now", Status: "passed", UpdatedAt: now.Format(time.RFC3339)},
		{ID: "passed-old", Status: "passed", UpdatedAt: now.Add(-3 * time.Hour).Format(time.RFC3339)},
		{ID: "cancelled-undated", Status: "cancelled"},
	}
}

func runIDs(runs []Run) string {
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.ID
	}
	return strings.Join(out, ",")
}

// TestEachRunFilterAdmitsItsOwnSegment mirrors TestEachPRFilterAdmitsItsOwnSegment: active is the
// union of everything open with everything just finished.
func TestEachRunFilterAdmitsItsOwnSegment(t *testing.T) {
	want := map[RunFilter]string{
		RunFilterOpen:   "queued,running",
		RunFilterClosed: "passed-now,passed-old,cancelled-undated",
		RunFilterAll:    "queued,running,passed-now,passed-old,cancelled-undated",
		RunFilterActive: "queued,running,passed-now",
	}
	for f, w := range want {
		if got := runIDs(FilterRuns(f, queue())); got != w {
			t.Errorf("filter %q admitted %q, want %q", f, got, w)
		}
	}
}

// TestAnUndatedRunIsNeverRecent mirrors the PR/task version.
func TestAnUndatedRunIsNeverRecent(t *testing.T) {
	undated := Run{ID: "x", Status: "cancelled"}
	if RunChangedWithin(undated, ActiveWindow) {
		t.Error("a run with no timestamp cannot be known to be recent")
	}
	if MatchesRunFilter(RunFilterActive, undated) {
		t.Error("an undated cancelled run must not show under active")
	}
	if !MatchesRunFilter(RunFilterClosed, undated) || !MatchesRunFilter(RunFilterAll, undated) {
		t.Error("it must still be reachable under closed and all")
	}
}

// TestParseRunFilterNamesTheWholeSet mirrors the PR/task version.
func TestParseRunFilterNamesTheWholeSet(t *testing.T) {
	for _, f := range RunFilters {
		got, err := ParseRunFilter(string(f))
		if err != nil || got != f {
			t.Errorf("ParseRunFilter(%q) = %q, %v", f, got, err)
		}
	}
	_, err := ParseRunFilter("opne")
	if err == nil {
		t.Fatal("an unknown filter must be rejected, not silently accepted")
	}
	for _, f := range RunFilters {
		if !strings.Contains(err.Error(), string(f)) {
			t.Errorf("the error should name %q: %v", f, err)
		}
	}
}

// TestNextRunFilterCyclesAndWraps mirrors the PR/task version.
func TestNextRunFilterCyclesAndWraps(t *testing.T) {
	f := RunFilters[0]
	for i := range RunFilters {
		next := NextRunFilter(f)
		if want := RunFilters[(i+1)%len(RunFilters)]; next != want {
			t.Fatalf("after %q the cycle gave %q, want %q", f, next, want)
		}
		f = next
	}
	if got := NextRunFilter("nonsense"); got != RunFilters[0] {
		t.Errorf("an unknown filter should rejoin at %q, got %q", RunFilters[0], got)
	}
}
