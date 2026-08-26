package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestRunsTabDefaultsToActive: the section defaults the same way Tasks and PRs do.
func TestRunsTabDefaultsToActive(t *testing.T) {
	m := newModel(nil, nil, "")
	if m.runFilter != api.RunFilterActive {
		t.Errorf("runFilter default = %q, want active", m.runFilter)
	}
}

// scopedModel is a model whose active repo resolves to a real tag — inScope only narrows
// anything down once currentRepo() can name one; a bare newModel() leaves it "" and shows
// everything, which would make these tests pass for the wrong reason.
func scopedModel() model {
	m := newModel(nil, nil, "/repo")
	m.state.Projects = []api.Project{{Path: "/repo", Tag: "mine"}}
	return m
}

// TestRunRowsRespectsScopeAndFilter mirrors the PR-tab equivalent: a run outside the active
// repo (scoped on) is hidden, and a closed run with no recent change is hidden under active.
func TestRunRowsRespectsScopeAndFilter(t *testing.T) {
	m := scopedModel()
	m.state.Runs = []api.Run{
		{ID: "run-mine", Project: "mine", Status: "queued", Agent: "bombur", Command: "go test"},
		{ID: "run-other-repo", Project: "elsewhere", Status: "queued", Agent: "nori", Command: "go build"},
		{ID: "run-old", Project: "mine", Status: "cancelled", Agent: "nori", Command: "go vet"},
	}
	txt := strings.Join(rowTexts(m.runRows()), "\n")
	if !strings.Contains(txt, "run-mine") {
		t.Error("a queued run in the active repo must show")
	}
	if strings.Contains(txt, "run-other-repo") {
		t.Error("a run in a different repo must be hidden under the repo scope")
	}
	if strings.Contains(txt, "run-old") {
		t.Error("a cancelled run with no recent change must be hidden under the active default")
	}
}

// TestRunsTabCount mirrors the PRs tabCount case: queued+running, scoped.
func TestRunsTabCount(t *testing.T) {
	m := scopedModel()
	m.state.Runs = []api.Run{
		{ID: "a", Project: "mine", Status: "queued"},
		{ID: "b", Project: "mine", Status: "running"},
		{ID: "c", Project: "mine", Status: "passed"},
		{ID: "d", Project: "elsewhere", Status: "queued"},
	}
	if got := m.tabCount(tuiSection{Key: "runs"}); got != 2 {
		t.Errorf("runs tab count = %d, want 2 (queued+running, in scope)", got)
	}
}

// TestRunRowsShowTook: the list column reads blank for a run that has not started, and the elapsed
// span once it has — the same figure api.RunTook derives, not a second copy of the arithmetic.
func TestRunRowsShowTook(t *testing.T) {
	m := scopedModel()
	m.runFilter = api.RunFilterAll // took, not filtering, is under test
	m.state.Runs = []api.Run{
		{ID: "run-queued", Project: "mine", Status: "queued", Command: "go test"},
		{ID: "run-done", Project: "mine", Status: "passed", Command: "go build",
			StartedAt: "2026-01-01T00:00:00Z", FinishedAt: "2026-01-01T00:05:00Z"},
	}
	rows := rowTexts(m.runRows())
	txt := strings.Join(rows, "\n")
	if !strings.Contains(txt, "5m0s") {
		t.Errorf("a finished run's row should show its took, got:\n%s", txt)
	}
	for _, line := range rows {
		if strings.Contains(line, "run-queued") && strings.Contains(line, "0s") {
			t.Errorf("a queued run's took should be blank, not a zero duration, got %q", line)
		}
	}
}

// TestRunItemsShowTookOnlyOnceItHasOne: the detail pane names a run's took beside its other timing
// fields, but only once RunTook has an answer — a queued run has taken no time to report.
func TestRunItemsShowTookOnlyOnceItHasOne(t *testing.T) {
	m := scopedModel()
	m.tab = 5                      // Runs
	m.runFilter = api.RunFilterAll // took, not filtering, is under test
	m.state.Runs = []api.Run{
		{ID: "run-done", Project: "mine", Status: "passed", Command: "go build",
			StartedAt: "2026-01-01T00:00:00Z", FinishedAt: "2026-01-01T00:05:00Z"},
	}
	m.reclamp()
	m.selectRow("run-done")
	m.runDetail = api.RunDetail{Run: m.state.Runs[0]}
	txt := strings.Join(itemTexts(m.runItems()), "\n")
	if !strings.Contains(txt, "took:     5m0s") {
		t.Errorf("the detail pane should name the run's took, got:\n%s", txt)
	}

	m.state.Runs[0] = api.Run{ID: "run-done", Project: "mine", Status: "queued", Command: "go build"}
	m.runDetail = api.RunDetail{Run: m.state.Runs[0]}
	if strings.Contains(strings.Join(itemTexts(m.runItems()), "\n"), "took:") {
		t.Error("a queued run's detail should not carry a took line at all")
	}
}

// TestRunStatusLabelShowsQueuePosition: "queued" alone never says where in line a run is.
func TestRunStatusLabelShowsQueuePosition(t *testing.T) {
	if got, want := runStatusLabel(api.Run{Status: "queued", Position: 3}), "queued(#3)"; got != want {
		t.Errorf("runStatusLabel = %q, want %q", got, want)
	}
	if got, want := runStatusLabel(api.Run{Status: "running"}), "running"; got != want {
		t.Errorf("runStatusLabel = %q, want %q", got, want)
	}
}
