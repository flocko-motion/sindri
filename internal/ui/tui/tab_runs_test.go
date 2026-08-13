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

// TestRunStatusLabelShowsQueuePosition: "queued" alone never says where in line a run is.
func TestRunStatusLabelShowsQueuePosition(t *testing.T) {
	if got, want := runStatusLabel(api.Run{Status: "queued", Position: 3}), "queued(#3)"; got != want {
		t.Errorf("runStatusLabel = %q, want %q", got, want)
	}
	if got, want := runStatusLabel(api.Run{Status: "running"}), "running"; got != want {
		t.Errorf("runStatusLabel = %q, want %q", got, want)
	}
}
