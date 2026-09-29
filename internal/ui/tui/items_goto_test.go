package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// gotoItemBoard is a task closed and filtered out of the default Tasks view, and a PR merged and
// filtered out of the default PRs view — the two axes gotoItem must widen to land on either.
func gotoItemBoard() api.BoardState {
	return api.BoardState{
		Projects: []api.Project{{Tag: "here", Path: "/r/here"}},
		Tasks:    []api.Task{{ID: "sd-closed", Title: "old bug", Status: "closed"}},
		PRs:      []api.PR{{ID: "pr-merged", Task: "sd-closed", Agent: "nori", Project: "here", Status: "merged"}},
	}
}

func gotoItemModel() model {
	m := newModel(nil, nil, "/r/here")
	m.w, m.h = 120, 30
	m.state = gotoItemBoard()
	m.reclamp()
	return m
}

// TestGotoItemWidensAHiddenTaskAndSaysSo: landing on a filtered-out task must widen the filter
// that hides it and say so — never leave the cursor wherever it happened to sit.
func TestGotoItemWidensAHiddenTaskAndSaysSo(t *testing.T) {
	m := gotoItemModel()
	m.tab = 1 // start elsewhere, so a same-tab no-op can't hide behind "it was already there"
	if m.filter == api.FilterAll {
		t.Fatal("precondition: the task must start hidden by a non-widest filter")
	}
	m.gotoItem("task", "sd-closed")

	if m.tab != 0 {
		t.Fatalf("should have landed on Tasks, got tab %d", m.tab)
	}
	if got := m.selID(); got != "sd-closed" {
		t.Errorf("should have selected sd-closed, got %q", got)
	}
	if m.filter != api.FilterAll {
		t.Errorf("the filter hiding it should have been widened, got %q", m.filter)
	}
	if !strings.Contains(m.flash, "sd-closed") || !strings.Contains(m.flash, "widened") {
		t.Errorf("the flash should say the target was hidden and the view was widened, got %q", m.flash)
	}
}

// TestGotoItemWidensAHiddenSearchAndSaysSo: a committed search that excludes the target is its own
// axis on Tasks (sd-a130a5), and must be cleared too — otherwise the message claims the task can't
// be found while it sits right there, only excluded by the search term, filter untouched.
func TestGotoItemWidensAHiddenSearchAndSaysSo(t *testing.T) {
	m := gotoItemModel()
	m.state.Tasks = append(m.state.Tasks, api.Task{ID: "sd-visible", Title: "wire the thing", Status: "open"})
	m.taskSearch = "does-not-match-anything"
	m.tab = 1 // start elsewhere, so a same-tab no-op can't hide behind "it was already there"

	m.gotoItem("task", "sd-visible")

	if got := m.selID(); got != "sd-visible" {
		t.Errorf("should have selected sd-visible, got %q", got)
	}
	if m.taskSearch != "" {
		t.Errorf("the search excluding it should have been cleared, got %q", m.taskSearch)
	}
	if !strings.Contains(m.flash, "sd-visible") || !strings.Contains(m.flash, "widened") {
		t.Errorf("the flash should say the target was hidden and the view was widened, got %q", m.flash)
	}
}

// TestGotoItemWidensAHiddenPRAndSaysSo mirrors the task case for the PR filter, from the same-tab
// direction: the target is on the tab already open, so a bug here shows up as the key doing
// nothing rather than as a wrong row.
func TestGotoItemWidensAHiddenPRAndSaysSo(t *testing.T) {
	m := gotoItemModel()
	m.tab = 2
	if m.prFilter == api.PRFilterAll {
		t.Fatal("precondition: the PR must start hidden by a non-widest filter")
	}
	m.gotoItem("pr", "pr-merged")

	if got := m.selID(); got != "pr-merged" {
		t.Errorf("should have selected pr-merged, got %q", got)
	}
	if m.prFilter != api.PRFilterAll {
		t.Errorf("the PR filter hiding it should have been widened, got %q", m.prFilter)
	}
	if !strings.Contains(m.flash, "pr-merged") || !strings.Contains(m.flash, "widened") {
		t.Errorf("the flash should say the target was hidden and the view was widened, got %q", m.flash)
	}
}

// TestGotoItemOnAVisibleTargetWidensNothing: the common case must not disturb the filter or say
// anything was widened when the target was already in view.
func TestGotoItemOnAVisibleTargetWidensNothing(t *testing.T) {
	m := gotoItemModel()
	m.state.Tasks = append(m.state.Tasks, api.Task{ID: "sd-open", Title: "open one", Status: "open"})
	m.gotoItem("task", "sd-open")

	if m.filter != api.FilterActive {
		t.Errorf("a visible target must not widen the filter, got %q", m.filter)
	}
	if strings.Contains(m.flash, "widened") {
		t.Errorf("nothing was hidden, so nothing should claim to have been widened, got %q", m.flash)
	}
	if got := m.selID(); got != "sd-open" {
		t.Errorf("should have selected sd-open, got %q", got)
	}
}

// TestGotoItemOnAnIDThatDoesNotExistSaysSo: widening everything still can't find a row that was
// deleted from the board entirely — it must say so rather than silently leaving a stale selection.
func TestGotoItemOnAnIDThatDoesNotExistSaysSo(t *testing.T) {
	m := gotoItemModel()
	m.gotoItem("task", "sd-nonexistent")

	if got := m.selID(); got == "sd-nonexistent" {
		t.Fatal("there is no such row to select")
	}
	if !strings.Contains(m.flash, "sd-nonexistent") {
		t.Errorf("the flash should name what could not be found, got %q", m.flash)
	}
}
