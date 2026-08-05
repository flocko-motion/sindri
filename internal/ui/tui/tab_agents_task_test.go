package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestAgentDetailShowsTaskTitle: the agent detail pane used to show only a task's bare id ("what
// is it working on?" answered with nothing a human recognizes) — both the live Agents-tab pane
// (agentItems) and the shared renderer used by the peek modal and yank (agentDetailFor) must now
// show the title alongside it.
func TestAgentDetailShowsTaskTitle(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 1 // Agents
	m.state = api.BoardState{
		Agents: []api.AgentView{{Name: "brokkr", Role: "worker", Status: "working", Task: "td-42"}},
		Tasks:  []api.Task{{ID: "td-42", Title: "fix the flaky test"}},
	}
	m.cursor[1] = 0
	m.reclamp()

	items := m.agentItems()
	var taskLine string
	for _, it := range items {
		if strings.HasPrefix(it.text, "task:") {
			taskLine = it.text
		}
	}
	if !strings.Contains(taskLine, "td-42") || !strings.Contains(taskLine, "fix the flaky test") {
		t.Fatalf("agentItems task line = %q, want both the id and the title", taskLine)
	}

	lines := strings.Join(m.agentDetailFor(m.state.Agents[0]), "\n")
	if !strings.Contains(lines, "td-42") || !strings.Contains(lines, "fix the flaky test") {
		t.Fatalf("agentDetailFor should show the title too:\n%s", lines)
	}
}

// TestAgentTaskCrossRefStillNavigable: the fix must not smuggle the title into the value that
// ENTER/g act on — that value has to stay the bare task id, or jumping to it breaks.
func TestAgentTaskCrossRefStillNavigable(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 1
	m.state = api.BoardState{
		Agents: []api.AgentView{{Name: "brokkr", Role: "worker", Task: "td-42"}},
		Tasks:  []api.Task{{ID: "td-42", Title: "fix the flaky test"}},
	}
	m.cursor[1] = 0
	m.reclamp()

	var taskItem metaItem
	found := false
	for _, it := range m.agentItems() {
		if it.kind == "task" {
			taskItem, found = it, true
		}
	}
	if !found {
		t.Fatal("expected an actionable task cross-reference")
	}
	if taskItem.value != "td-42" {
		t.Fatalf("cross-reference value = %q, want the bare id %q", taskItem.value, "td-42")
	}
}

// TestAgentDetailFallsBackToBareID: a task not in the board's cache (another project, or
// closed/scrapped since) must degrade to the bare id, not blank out or crash.
func TestAgentDetailFallsBackToBareID(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 1
	m.state = api.BoardState{
		Agents: []api.AgentView{{Name: "brokkr", Role: "worker", Task: "td-99"}},
		// No matching task in m.state.Tasks.
	}
	m.cursor[1] = 0
	m.reclamp()

	var taskLine string
	for _, it := range m.agentItems() {
		if strings.HasPrefix(it.text, "task:") {
			taskLine = it.text
		}
	}
	if !strings.Contains(taskLine, "td-99") {
		t.Fatalf("should still show the bare id when the title is unknown, got %q", taskLine)
	}
}

// TestAgentDetailNoTaskShowsDash: an idle agent's "task:" line stays "-", the existing convention
// for an absent value.
func TestAgentDetailNoTaskShowsDash(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 1
	m.state = api.BoardState{Agents: []api.AgentView{{Name: "brokkr", Role: "worker", Status: "idle"}}}
	m.cursor[1] = 0
	m.reclamp()

	var taskLine string
	for _, it := range m.agentItems() {
		if strings.HasPrefix(it.text, "task:") {
			taskLine = it.text
		}
	}
	if !strings.Contains(taskLine, "-") {
		t.Fatalf("an idle agent's task line should be a dash, got %q", taskLine)
	}
	if taskLine != "task:      -" {
		t.Errorf("task line = %q, want %q", taskLine, "task:      -")
	}
}
