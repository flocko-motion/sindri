package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// agentDetail renders the Agents side pane for the selected agent.
func agentDetail(m model) string { return strings.Join(itemTexts(m.agentItems()), "\n") }

// TestTheDetailPaneNamesTheHeldFeature: between subtasks a worker holds a feature and no task, so
// the pane's task line is a dash — and with nothing else naming the feature it read as an agent
// holding nothing, while that feature was in fact gating every verb it had.
func TestTheDetailPaneNamesTheHeldFeature(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 1
	m.state = api.BoardState{
		Agents: []api.AgentView{{Name: "dain", Role: "worker", Status: "idle", Feature: "os-057e90"}},
		Tasks:  []api.Task{{ID: "os-057e90", Title: "separate-hub-from-frontends"}},
	}
	m.reclamp()

	got := agentDetail(m)
	if !strings.Contains(got, "os-057e90") {
		t.Errorf("the pane should name the held feature:\n%s", got)
	}
	// And with its title, like every other task reference in the UI.
	if !strings.Contains(got, "separate-hub-from-frontends") {
		t.Errorf("the feature should carry its title:\n%s", got)
	}
	// It is a cross-reference, so enter opens it — the same as the task line.
	var focusable bool
	for _, it := range m.agentActionable() {
		if it.kind == "task" && it.value == "os-057e90" {
			focusable = true
		}
	}
	if !focusable {
		t.Error("the feature should be focusable, like the task and PR cross-references")
	}
}

// TestTheDetailPaneShowsBothWhenWorkingASubtask: on a subtask, the pane names the subtask AND the
// feature it belongs to — the tree is the context, and one line without the other loses half of it.
func TestTheDetailPaneShowsBothWhenWorkingASubtask(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 1
	m.state = api.BoardState{
		Agents: []api.AgentView{{Name: "dain", Role: "worker", Status: "working", Task: "td-6a93cd", Feature: "os-057e90"}},
	}
	m.reclamp()

	got := agentDetail(m)
	for _, want := range []string{"td-6a93cd", "os-057e90"} {
		if !strings.Contains(got, want) {
			t.Errorf("the pane should name %q:\n%s", want, got)
		}
	}
}

// TestAPlainWorkerShowsNoFeature: the line is a placeholder for an agent holding no feature, in
// keeping with the other cross-references, and never invents one.
func TestAPlainWorkerShowsNoFeature(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 1
	m.state = api.BoardState{
		Agents: []api.AgentView{{Name: "bombur", Role: "worker", Status: "working", Task: "td-1"}},
	}
	m.reclamp()

	for _, it := range m.agentActionable() {
		if it.kind == "task" && it.value == "" {
			t.Error("an empty feature must not be focusable")
		}
	}
	if got := agentDetail(m); !strings.Contains(got, "feature:   -") {
		t.Errorf("the feature line should be a placeholder:\n%s", got)
	}
}
