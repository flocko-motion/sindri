package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestAgentRowShowsReviewedPR: a reviewer holds no task, so its work cell would be
// empty ("-"). The core fills AgentView.PR with the PR it's reviewing; the list row
// must fall back to that, so the agent list tells what a reviewer is reviewing.
func TestAgentRowShowsReviewedPR(t *testing.T) {
	m := newModel(nil, nil, "")
	m.scopeRepo = false // global scope, so the row isn't filtered out by repo tag
	m.state = api.BoardState{Agents: []api.AgentView{
		{Name: "dvalin", Role: "reviewer", Status: "reviewing", Task: "", PR: "pr-td-9"},
		{Name: "eitri", Role: "worker", Status: "working", Task: "td-3"},
	}}

	rows := m.agentRows()
	find := func(id string) string {
		for _, r := range rows {
			if r.id == id {
				return r.text
			}
		}
		return ""
	}

	if got := find("dvalin"); !strings.Contains(got, "pr-td-9") {
		t.Fatalf("reviewer row should show the reviewed PR, got %q", got)
	}
	// The worker still shows its task (unchanged behaviour).
	if got := find("eitri"); !strings.Contains(got, "td-3") {
		t.Fatalf("worker row should still show its task, got %q", got)
	}
}

// TestAgentRowShowsWhatTaskTheReviewedPRIsFor: the PR id alone ("pr-td-9") says nothing a human
// recognizes — the same gap taskLabel closed for the task column itself. The row must also name
// the task that PR is for.
func TestAgentRowShowsWhatTaskTheReviewedPRIsFor(t *testing.T) {
	m := newModel(nil, nil, "")
	m.scopeRepo = false
	m.state = api.BoardState{
		Agents: []api.AgentView{{Name: "dvalin", Role: "reviewer", Status: "reviewing", Task: "", PR: "pr-td-9"}},
		PRs:    []api.PR{{ID: "pr-td-9", Task: "td-9"}},
		Tasks:  []api.Task{{ID: "td-9", Title: "fix the flaky test"}},
	}

	var got string
	for _, r := range m.agentRows() {
		if r.id == "dvalin" {
			got = r.text
		}
	}
	if !strings.Contains(got, "pr-td-9") {
		t.Fatalf("the reviewed PR should still show, got %q", got)
	}
	if !strings.Contains(got, "td-9") || !strings.Contains(got, "fix the flaky test") {
		t.Fatalf("the row should also name the task the PR is for, got %q", got)
	}
}

// TestAgentDetailShowsWhatTaskTheReviewedPRIsFor: the same gap in the detail pane's "task:" line —
// a reviewer's own Task is always "", so without this fallback the pane read as holding nothing.
func TestAgentDetailShowsWhatTaskTheReviewedPRIsFor(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 1
	m.state = api.BoardState{
		Agents: []api.AgentView{{Name: "dvalin", Role: "reviewer", Status: "reviewing", Task: "", PR: "pr-td-9"}},
		PRs:    []api.PR{{ID: "pr-td-9", Task: "td-9"}},
		Tasks:  []api.Task{{ID: "td-9", Title: "fix the flaky test"}},
	}
	m.cursor[1] = 0
	m.reclamp()

	var taskLine string
	for _, it := range m.agentItems() {
		if strings.HasPrefix(it.text, "task:") {
			taskLine = it.text
		}
	}
	if !strings.Contains(taskLine, "td-9") || !strings.Contains(taskLine, "fix the flaky test") {
		t.Fatalf("the task line should name what the reviewed PR is for, got %q", taskLine)
	}

	// It stays a real cross-reference: ENTER/g goes to the task, not the PR.
	var taskItem metaItem
	found := false
	for _, it := range m.agentItems() {
		if it.kind == "task" {
			taskItem, found = it, true
		}
	}
	if !found || taskItem.value != "td-9" {
		t.Fatalf("expected an actionable task cross-reference to td-9, got %+v (found=%v)", taskItem, found)
	}
}
