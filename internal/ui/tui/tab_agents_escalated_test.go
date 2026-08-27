package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// escalatedModel is the Agents tab on one escalated agent.
func escalatedModel(question string) *model {
	m := newModel(nil, nil, "")
	m.tab, m.scopeRepo = 1, false
	m.state = api.BoardState{Agents: []api.AgentView{{
		Name: "dvalin", Role: "worker", Status: api.StatusEscalated, Task: "sd-1", Escalation: question,
	}}}
	return &m
}

// TestTheEscalatedQuestionIsReadableInTheDetail: the status word says an agent is waiting on the user,
// and the question says what for. It belongs in the detail so several escalations can be triaged
// before deciding which to sit down with — attaching to each pane in turn cannot do that.
func TestTheEscalatedQuestionIsReadableInTheDetail(t *testing.T) {
	const q = "drop the two callers or keep both?"
	m := escalatedModel(q)
	if joined := strings.Join(itemTexts(m.agentItems()), "\n"); !strings.Contains(joined, q) {
		t.Errorf("the agent detail omits the question it is stopped on:\n%s", joined)
	}
	// And in the plain detail lines too, which are what the modal shows and `Y` copies.
	if joined := strings.Join(m.agentDetailLines(), "\n"); !strings.Contains(joined, q) {
		t.Errorf("the detail lines omit the question:\n%s", joined)
	}
}

// TestTheEscalationLineReleasesTheAgent: the user's own clear, reached where the question is read
// rather than through a hotkey of its own — it only means anything on an escalated agent, and this is
// the one place such an agent is looked at. ENTER opens a form rather than committing, because what
// the agent stopped for is a QUESTION and the release is where its answer belongs.
func TestTheEscalationLineReleasesTheAgent(t *testing.T) {
	m := escalatedModel("keep both?")
	var esc metaItem
	for _, it := range m.agentItems() {
		if strings.HasPrefix(it.text, "escalated:") {
			esc = it
		}
	}
	if esc.kind != "resume" || esc.value != "dvalin" {
		t.Fatalf("the escalation line should be actionable as a resume, got kind=%q value=%q", esc.kind, esc.value)
	}
	for i, it := range m.agentActionable() {
		if it.kind == "resume" {
			m.focus, m.rightCursor = focusItems, i
		}
	}
	if m.focus != focusItems {
		t.Fatal("the escalation line is not reachable by the right-column cursor")
	}
	m.onKey("enter")
	if !m.form.active || !strings.Contains(m.form.title, "dvalin") {
		t.Errorf("ENTER should open the resume form, got %+v", m.form)
	}
	if len(m.form.fields) != 1 {
		t.Errorf("the form carries the answer to send with the release, got %d field(s)", len(m.form.fields))
	}
}

// TestAnAgentWithNoEscalationHasNoSuchLine: the field exists only where it says something. Shown
// empty it would read as an agent that had asked something nobody recorded.
func TestAnAgentWithNoEscalationHasNoSuchLine(t *testing.T) {
	m := escalatedModel("")
	m.state.Agents[0].Status = "working"
	for _, it := range m.agentItems() {
		if strings.HasPrefix(it.text, "escalated:") {
			t.Errorf("an agent that has escalated nothing shows an escalation line: %q", it.text)
		}
	}
}
