package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub"
)

// TestAgentRowShowsReviewedPR: a reviewer holds no task, so its work cell would be
// empty ("-"). The core fills AgentView.PR with the PR it's reviewing; the list row
// must fall back to that, so the agent list tells what a reviewer is reviewing.
func TestAgentRowShowsReviewedPR(t *testing.T) {
	m := newModel(nil, nil, "")
	m.scopeRepo = false // global scope, so the row isn't filtered out by repo tag
	m.state = hub.BoardState{Agents: []hub.AgentView{
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
