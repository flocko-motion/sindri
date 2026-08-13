package api

import (
	"strings"
	"testing"
	"time"
)

// backlog is one task per case the filters have to separate.
func backlog() []Task {
	now := time.Now().UTC()
	return []Task{
		{ID: "open", Status: "open"},
		{ID: "in_progress", Status: "in_progress"},
		{ID: "open-undated", Status: "open"},
		{ID: "closed-now", Status: "closed", UpdatedAt: now.Format(time.RFC3339)},
		{ID: "closed-old", Status: "closed", UpdatedAt: now.Add(-3 * time.Hour).Format(time.RFC3339)},
		{ID: "closed-undated", Status: "closed"},
		{ID: "merged-now", Status: "merged", UpdatedAt: now.Format(time.RFC3339)},
	}
}

func ids(tasks []Task) string {
	out := make([]string, len(tasks))
	for i, t := range tasks {
		out[i] = t.ID
	}
	return strings.Join(out, ",")
}

// TestEachFilterAdmitsItsOwnSegment walks all four against one backlog. Active is the one worth
// reading twice: it is a UNION of everything open with everything just closed, so work finished a
// moment ago is still on screen while last week's is not.
func TestEachFilterAdmitsItsOwnSegment(t *testing.T) {
	want := map[TaskFilter]string{
		FilterOpen:   "open,in_progress,open-undated",
		FilterClosed: "closed-now,closed-old,closed-undated,merged-now",
		FilterAll:    "open,in_progress,open-undated,closed-now,closed-old,closed-undated,merged-now",
		FilterActive: "open,in_progress,open-undated,closed-now,merged-now",
	}
	for f, w := range want {
		if got := ids(FilterTasks(f, backlog())); got != w {
			t.Errorf("filter %q admitted %q, want %q", f, got, w)
		}
	}
}

// TestAnUndatedTaskIsNeverRecent: without a timestamp there is no evidence of recency, so a closed
// task carrying none shows under closed and all, and never under active. Every source is expected
// to date its tasks for exactly this reason.
func TestAnUndatedTaskIsNeverRecent(t *testing.T) {
	undated := Task{ID: "x", Status: "closed"}
	if ChangedWithin(undated, ActiveWindow) {
		t.Error("a task with no timestamp cannot be known to be recent")
	}
	if MatchesFilter(FilterActive, undated) {
		t.Error("an undated closed task must not show under active")
	}
	if !MatchesFilter(FilterClosed, undated) || !MatchesFilter(FilterAll, undated) {
		t.Error("it must still be reachable under closed and all")
	}
	// A malformed timestamp is no better evidence than none.
	if ChangedWithin(Task{UpdatedAt: "yesterday"}, ActiveWindow) {
		t.Error("an unparsable timestamp must not read as recent")
	}
}

// TestParseTaskFilterNamesTheWholeSet: a typo'd flag has to say what the valid values are, since
// the four words are the entire interface.
func TestParseTaskFilterNamesTheWholeSet(t *testing.T) {
	for _, f := range TaskFilters {
		got, err := ParseTaskFilter(string(f))
		if err != nil || got != f {
			t.Errorf("ParseTaskFilter(%q) = %q, %v", f, got, err)
		}
	}
	_, err := ParseTaskFilter("opne")
	if err == nil {
		t.Fatal("an unknown filter must be rejected, not silently accepted")
	}
	for _, f := range TaskFilters {
		if !strings.Contains(err.Error(), string(f)) {
			t.Errorf("the error should name %q: %v", f, err)
		}
	}
}

// TestUnknownFilterShowsEverything: nothing should be able to produce a listing that reads as an
// empty backlog when the backlog is full.
func TestUnknownFilterShowsEverything(t *testing.T) {
	if got := len(FilterTasks(TaskFilter("nonsense"), backlog())); got != len(backlog()) {
		t.Errorf("an unrecognised filter admitted %d tasks, want all %d", got, len(backlog()))
	}
}

// TestNextTaskFilterCyclesAndWraps: the cycle a front-end walks visits each filter once and returns
// to where it began, and an unknown filter rejoins the set rather than sticking outside it.
func TestNextTaskFilterCyclesAndWraps(t *testing.T) {
	f := TaskFilters[0]
	for i := range TaskFilters {
		next := NextTaskFilter(f)
		if want := TaskFilters[(i+1)%len(TaskFilters)]; next != want {
			t.Fatalf("after %q the cycle gave %q, want %q", f, next, want)
		}
		f = next
	}
	if got := NextTaskFilter("nonsense"); got != TaskFilters[0] {
		t.Errorf("an unknown filter should rejoin at %q, got %q", TaskFilters[0], got)
	}
}
