package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestAttachedOpenPR: a task's open PR (same project) is detected, while merged and
// scrapped PRs (already off the board) and other-project PRs are never offered.
func TestAttachedOpenPR(t *testing.T) {
	m := newModel(nil, nil, "")
	_, tag := m.currentRepo() // match the active-repo tag attachedOpenPR filters on
	m.state = api.BoardState{PRs: []api.PR{
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
	m.state = api.BoardState{PRs: []api.PR{{ID: "pr-td-1", Task: "td-1", Project: tag, Status: "approved"}}}

	m.openScrapChoice("td-1")
	if !m.choice.active || len(m.choice.options) != 3 {
		t.Fatalf("task with an open PR should give a 3-option scrap modal, got %v", m.choice.options)
	}
	m.openScrapChoice("td-2") // no PR attached
	if !m.choice.active || len(m.choice.options) != 2 {
		t.Fatalf("task without a PR should give the 2-option confirm, got %v", m.choice.options)
	}
}

// TestScrapChoiceOffersTheSubtasks: a task with children must offer to take the whole
// hierarchy under it — and, when anything in that set has an open PR, to take those
// too. Each wider reach is its own option, so the task-only scrap stays available.
func TestScrapChoiceOffersTheSubtasks(t *testing.T) {
	m := newModel(nil, nil, "")
	_, tag := m.currentRepo()
	m.state = api.BoardState{
		Tasks: []api.Task{
			{ID: "td-1"},
			{ID: "td-2", ParentID: "td-1"},
			{ID: "td-3", ParentID: "td-2"},
		},
		PRs: []api.PR{{ID: "pr-td-3", Task: "td-3", Project: tag, Status: "submitted"}},
	}

	m.openScrapChoice("td-1")
	want := []string{"cancel", "scrap task only", "scrap task + 2 subtasks", "scrap task + 2 subtasks + 1 PR"}
	if got := m.choice.options; !slices.Equal(got, want) {
		t.Fatalf("scrap modal options = %v, want %v", got, want)
	}
	if got := m.choice.values; !slices.Equal(got, []string{"cancel", "task", "tree", "treepr"}) {
		t.Fatalf("scrap modal values = %v", got)
	}
	if !strings.Contains(m.choice.title, "2 subtasks") {
		t.Errorf("the title should say what is attached, got %q", m.choice.title)
	}

	// The child's own PR is its own affair: scrapping the child (a leaf) offers the
	// plain task/PR pair, with no subtree option.
	m.openScrapChoice("td-3")
	if got := m.choice.values; !slices.Equal(got, []string{"cancel", "task", "taskpr"}) {
		t.Errorf("a leaf with a PR = %v, want the task/PR pair", got)
	}
}

// TestScrapPRChoiceOffersItsTask: scrapping a PR whose task is still open must offer to take the
// task too — without that, the task sits open and claimable, so the same work gets redone right
// after the PR that did it is thrown away (-> sd-1c3242). A PR whose task is already done, or one
// with none at all, gets the plain confirm instead.
func TestScrapPRChoiceOffersItsTask(t *testing.T) {
	m := newModel(nil, nil, "")
	m.state = api.BoardState{
		Tasks: []api.Task{{ID: "td-1", Status: "open"}, {ID: "td-2", Status: "closed"}},
		PRs: []api.PR{
			{ID: "pr-td-1", Task: "td-1", Status: "submitted"},
			{ID: "pr-td-2", Task: "td-2", Status: "submitted"},
			{ID: "pr-td-3", Task: "", Status: "submitted"},
		},
	}

	m.openScrapPRChoice("pr-td-1")
	if got := m.choice.values; !slices.Equal(got, []string{"cancel", "pr", "prtask"}) {
		t.Fatalf("a PR with an open task = %v, want the PR/task pair", got)
	}
	if !strings.Contains(m.choice.options[2], "td-1") {
		t.Errorf("the task+PR option should name the task, got %v", m.choice.options)
	}

	m.openScrapPRChoice("pr-td-2")
	if got := m.choice.values; !slices.Equal(got, []string{"cancel", "pr"}) {
		t.Fatalf("a PR whose task is already done = %v, want the plain confirm", got)
	}

	m.openScrapPRChoice("pr-td-3")
	if got := m.choice.values; !slices.Equal(got, []string{"cancel", "pr"}) {
		t.Fatalf("a PR with no task = %v, want the plain confirm", got)
	}
}

// TestPRFilterHidesScrapped: a scrapped PR with no recent change is hidden under open and
// under the active default alike, appears under all, and is excluded by closed's opposite.
func TestPRFilterHidesScrapped(t *testing.T) {
	m := newModel(nil, nil, "")
	_, tag := m.currentRepo()
	m.state = api.BoardState{PRs: []api.PR{{ID: "pr-1", Project: tag, Status: "scrapped"}}}
	shown := func() bool {
		for _, r := range m.prRows() {
			if r.id == "pr-1" {
				return true
			}
		}
		return false
	}

	m.prFilter = api.PRFilterOpen
	if shown() {
		t.Error("scrapped must be hidden under open")
	}
	m.prFilter = api.PRFilterActive
	if shown() {
		t.Error("scrapped with no recent change must be hidden under the active default")
	}
	m.prFilter = api.PRFilterAll
	if !shown() {
		t.Error("scrapped must appear under all")
	}
	m.prFilter = api.PRFilterClosed
	if !shown() {
		t.Error("scrapped must appear under closed")
	}
}
