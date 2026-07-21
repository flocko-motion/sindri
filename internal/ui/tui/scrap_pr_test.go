package tui

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestAttachedOpenPR: a task's open PR (same project) is detected, while merged and
// scrapped PRs (already off the board) and other-project PRs are never offered.
func TestAttachedOpenPR(t *testing.T) {
	m := newModel(nil, nil, "")
	_, tag := m.currentRepo() // match the active-repo tag attachedOpenPR filters on
	m.state = hub.BoardState{PRs: []store.PR{
		{ID: "pr-td-1", Task: "td-1", Project: tag, Status: "approved"},
		{ID: "pr-td-2", Task: "td-2", Project: tag, Status: "merged"},
		{ID: "pr-td-3", Task: "td-3", Project: tag, Status: "scrapped"},
		{ID: "pr-td-4", Task: "td-4", Project: "other-repo", Status: "approved"},
	}}
	cases := map[string]string{"td-1": "pr-td-1", "td-2": "", "td-3": "", "td-4": "", "td-none": ""}
	for task, want := range cases {
		if got := m.attachedOpenPR(task); got != want {
			t.Errorf("attachedOpenPR(%q) = %q, want %q", task, got, want)
		}
	}
}

// TestScrapChoiceOffersPR: scrapping a task WITH an open PR yields the 3-way modal
// (task-only vs task+PR); without one it's the plain 2-way confirm.
func TestScrapChoiceOffersPR(t *testing.T) {
	m := newModel(nil, nil, "")
	_, tag := m.currentRepo()
	m.state = hub.BoardState{PRs: []store.PR{{ID: "pr-td-1", Task: "td-1", Project: tag, Status: "approved"}}}

	m.openScrapChoice("td-1")
	if !m.choice.active || len(m.choice.options) != 3 {
		t.Fatalf("task with an open PR should give a 3-option scrap modal, got %v", m.choice.options)
	}
	m.openScrapChoice("td-2") // no PR attached
	if !m.choice.active || len(m.choice.options) != 2 {
		t.Fatalf("task without a PR should give the 2-option confirm, got %v", m.choice.options)
	}
}

// TestPRFilterHidesScrapped: a scrapped PR is hidden in the default (unmerged) view
// like merged, appears under filter=all, and is not shown by the merged-only filter.
func TestPRFilterHidesScrapped(t *testing.T) {
	m := newModel(nil, nil, "")
	m.prFilter = prFilterUnmerged
	if m.prFilterShows("scrapped") {
		t.Error("scrapped must be hidden in the unmerged (default) view")
	}
	m.prFilter = prFilterAll
	if !m.prFilterShows("scrapped") {
		t.Error("scrapped must appear under filter=all")
	}
	m.prFilter = prFilterMerged
	if m.prFilterShows("scrapped") {
		t.Error("the merged-only filter must not show scrapped")
	}
}
