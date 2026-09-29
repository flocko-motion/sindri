package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// taskFleet: one agent working a subtask inside an epic, another on a standalone task, and an
// epic nobody is working.
func taskFleet() api.BoardState {
	return api.BoardState{
		Projects: []api.Project{{Tag: "one", Path: "/r/one"}},
		Tasks: []api.Task{
			{ID: "td-EPIC", Title: "Login feature"},
			{ID: "td-1", Title: "Form UI", ParentID: "td-EPIC"},
			{ID: "td-2", Title: "Session store", ParentID: "td-1"}, // nested
			{ID: "td-SOLO", Title: "Fix the glitch"},
			{ID: "td-COLD", Title: "Nobody's work"},
		},
		Agents: []api.AgentView{
			{Name: "dvalin", Project: "one", Status: "working", Task: "td-2"}, // deep in the epic
			{Name: "nori", Project: "one", Status: "idle", Task: "td-SOLO"},
			{Name: "galar", Project: "one", Status: "planning", Task: ""}, // holds nothing
		},
	}
}

// TestAttachFromTaskFindsThePackageHolder: a hierarchy is claimed whole and the agent's state
// names the SUBTASK it is on, so selecting the parent — the row that represents the work — must
// still reach the agent holding that tree.
func TestAttachFromTaskFindsThePackageHolder(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.state = taskFleet()

	for _, tc := range []struct{ task, want string }{
		{"td-2", "dvalin"},    // the exact subtask
		{"td-1", "dvalin"},    // its parent
		{"td-EPIC", "dvalin"}, // the top of the package
		{"td-SOLO", "nori"},   // a standalone task
	} {
		a, ok := m.agentOnTask(tc.task)
		if !ok || a.Name != tc.want {
			t.Errorf("%s: got %q ok=%v, want %s", tc.task, a.Name, ok, tc.want)
		}
	}
}

// TestAttachFromTaskFindsNobodyWhenIdle: a task no agent holds reports none, so the key can say
// so rather than attaching to whoever happens to be first.
func TestAttachFromTaskFindsNobodyWhenIdle(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.state = taskFleet()
	for _, id := range []string{"td-COLD", "", "td-nonexistent"} {
		if a, ok := m.agentOnTask(id); ok {
			t.Errorf("%q: expected no agent, got %q", id, a.Name)
		}
	}
}

// TestAgentOnTaskSurvivesACycle: the parent links come from a cache that could, in principle,
// contain a loop. Resolving one must not hang the UI.
func TestAgentOnTaskSurvivesACycle(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.state = api.BoardState{
		Tasks: []api.Task{
			{ID: "a", ParentID: "b"},
			{ID: "b", ParentID: "a"},
		},
		Agents: []api.AgentView{{Name: "dvalin", Task: "a"}},
	}
	if _, ok := m.agentOnTask("unrelated"); ok {
		t.Error("a cyclic chain must not match an unrelated task")
	}
	// And the agent's own task still resolves.
	if a, ok := m.agentOnTask("a"); !ok || a.Name != "dvalin" {
		t.Errorf("got %q ok=%v", a.Name, ok)
	}
}

// TestAttachFlashIsNotTruncatedWhenNothingIsSelected: selID() is "" on an empty list, and
// "no agent is working " + "" once read as a sentence cut off mid-word rather than an answer.
func TestAttachFlashIsNotTruncatedWhenNothingIsSelected(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.state = api.BoardState{Projects: []api.Project{{Tag: "one", Path: "/r/one"}}}
	m.tab = 0
	m.onKey(keyAttach)
	if strings.HasSuffix(m.flash, " ") {
		t.Errorf("flash should not trail off mid-word, got %q", m.flash)
	}
	if m.flash == "" {
		t.Error("attach with nothing selected should still set a flash")
	}
}

// TestTasksFooterOffersAttach: a binding nobody can see is a binding nobody uses, and the
// screenshot's footer line is truncated to the terminal — so assert on the string itself.
func TestTasksFooterOffersAttach(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	if got := m.footerFor(scopeTasks); !strings.Contains(got, "attach") {
		t.Errorf("the Tasks footer should offer attach:\n%s", got)
	}
	// Still offered where it already was — on a real roster agent (attach is now `when`-gated
	// against agentSelected, since it silently no-ops on an orphan container otherwise).
	m.tab = 1
	m.state = api.BoardState{Agents: []api.AgentView{{Name: "dvalin", Project: "repo", Status: "idle"}}}
	m.reclamp()
	if got := m.footerFor(scopeAgents); !strings.Contains(got, "attach") {
		t.Errorf("the Agents footer lost attach:\n%s", got)
	}
}
