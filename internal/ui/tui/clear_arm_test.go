package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// agentsBoard puts the cursor-selectable roster on the Agents tab.
func agentsBoard(agents ...api.AgentView) api.BoardState {
	return api.BoardState{Agents: agents, Projects: []api.Project{{Tag: "repo", Path: "/repo"}}}
}

// TestClearConfirmStatesWhenItLands: the modal is a yes/no on a destructive act, and the one thing
// it must carry is WHEN — taken from the agent's own state, never asked of the user.
func TestClearConfirmStatesWhenItLands(t *testing.T) {
	cases := []struct {
		name  string
		agent api.AgentView
		want  string
	}{
		{"idle worker clears at once", api.AgentView{Project: "repo", Name: "dvalin", Role: "worker", Status: "idle"}, "clears now"},
		{"working worker waits for its task", api.AgentView{Project: "repo", Name: "dvalin", Role: "worker", Task: "sd-123"}, "finishes sd-123"},
		{"reviewer waits for its verdict", api.AgentView{Project: "repo", Name: "nori", Role: "reviewer", PR: "pr-9"}, "verdict on pr-9"},
		{"between subtasks clears at once", api.AgentView{Project: "repo", Name: "dvalin", Role: "worker", Feature: "sd-epic"}, "clears now"},
	}
	for _, c := range cases {
		m := newModel(nil, nil, "")
		m.state = agentsBoard(c.agent)
		m.openClearContextChoice(c.agent)
		if !m.choice.active {
			t.Fatalf("%s: pressing C must always confirm — it destroys the session's whole memory", c.name)
		}
		if !strings.Contains(m.choice.title, c.want) {
			t.Errorf("%s: title %q should say %q", c.name, m.choice.title, c.want)
		}
	}
}

// TestCTogglesOffWithoutAModal: cancelling a destructive action is not itself destructive, so
// disarming stays reachable bare — no confirm, no prefix, friction with nothing behind it.
// Arming is the destructive direction: it commits (sd-6d0ff2), so it opens the confirm only from
// behind the space prefix.
func TestCTogglesOffWithoutAModal(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 1
	m.state = agentsBoard(api.AgentView{Project: "repo", Name: "dvalin", Role: "worker", Status: "idle", ClearArmed: true})
	m.reclamp()
	m.onKey(keyClearCtx) // disarm: bare, no prefix needed
	if m.choice.active {
		t.Errorf("disarming must not ask: %q", m.choice.title)
	}
	if !strings.Contains(m.flash, "cancelled") {
		t.Errorf("the user should be told the arming is gone, got %q", m.flash)
	}

	// And with nothing armed, the same bare key must do nothing — arming commits.
	m2 := newModel(nil, nil, "")
	m2.tab = 1
	m2.state = agentsBoard(api.AgentView{Project: "repo", Name: "dvalin", Role: "worker", Status: "idle"})
	m2.reclamp()
	m2.onKey(keyClearCtx)
	if m2.choice.active {
		t.Error("a bare committing key must not open the confirm")
	}
	m2.onKey(keyMenu)
	m2.onKey(keyClearCtx)
	if !m2.choice.active {
		t.Error("arming is the destructive direction and must be confirmed, from behind the prefix")
	}
}

// TestAnArmedClearIsVisible: a toggle you cannot see is worse than no toggle — there would be no
// way to tell an agent about to lose its session from one carrying on.
func TestAnArmedClearIsVisible(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 1
	armed := api.AgentView{Project: "repo", Name: "dvalin", Role: "worker", Status: "idle", ClearArmed: true}
	m.state = agentsBoard(armed)
	m.reclamp()

	rows := m.agentRows()
	if len(rows) != 1 || !strings.Contains(rows[0].text, clearGlyph) {
		t.Errorf("the row should carry the armed marker, got %+v", rows)
	}
	detail := strings.Join(itemTexts(m.agentItems()), "\n")
	if !strings.Contains(detail, "armed") || !strings.Contains(detail, "clears now") {
		t.Errorf("the detail should say it is armed and when it lands:\n%s", detail)
	}
	// Unarmed, neither says anything: the marker is a call to look, not furniture.
	m.state = agentsBoard(api.AgentView{Project: "repo", Name: "dvalin", Role: "worker", Status: "idle"})
	if rows := m.agentRows(); strings.Contains(rows[0].text, clearGlyph) {
		t.Errorf("an unarmed agent should carry no marker: %q", rows[0].text)
	}
	if detail := strings.Join(itemTexts(m.agentItems()), "\n"); strings.Contains(detail, "clear:") {
		t.Errorf("an unarmed agent's detail should not mention a clear:\n%s", detail)
	}
}
