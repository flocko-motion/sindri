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
		Name: "dvalin", Role: "worker", Status: api.StatusEscalated, NeedsUser: true, Task: "sd-1", Escalation: question,
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

// TestTheEscalationLineOnlyReads: the question is DETAIL, and releasing the agent is an ACTION, so
// the row that shows the question carries no release of its own. It used to, which put one of the
// tab's actions somewhere none of the others are — reachable only by focusing the pane and finding
// the row (-> keyResume, which is where it lives now).
func TestTheEscalationLineOnlyReads(t *testing.T) {
	m := escalatedModel("keep both?")
	for _, it := range m.agentActionable() {
		if strings.HasPrefix(it.text, "escalated:") {
			t.Errorf("the escalation line is actionable in the detail: kind=%q value=%q", it.kind, it.value)
		}
	}
}

// TestTheMenuActionReleasesTheAgent: with the other agent actions, behind the space prefix like every
// other committing one, and a FORM rather than a commit — what the agent stopped for is a question,
// and the release is where its answer belongs.
func TestTheMenuActionReleasesTheAgent(t *testing.T) {
	m := escalatedModel("keep both?")
	m.onKey(keyResume)
	if m.form.active {
		t.Fatal("the bare keystroke committed; it belongs behind the prefix")
	}
	m.onKey(keyMenu)
	m.onKey(keyResume)
	if !m.form.active || !strings.Contains(m.form.title, "dvalin") {
		t.Fatalf("%q after the prefix should open the resume form, got %+v", keyResume, m.form)
	}
	if len(m.form.fields) != 1 {
		t.Errorf("the form carries the answer to send with the release, got %d field(s)", len(m.form.fields))
	}
	// And the menu never offers it where there is no question to answer (-> agentEscalated), so the
	// letter reaches nothing even through the prefix.
	quiet := escalatedModel("")
	quiet.state.Agents[0].Status = "working"
	quiet.onKey(keyMenu)
	quiet.onKey(keyResume)
	if quiet.form.active {
		t.Error("an agent with no escalation was offered a release")
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
