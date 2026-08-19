package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// attentionScopeBoard: one working agent in the selected repo ("sin"), and in another repo one
// working agent and one that is stalled — the shape that produced the complaint, a marked handle over
// a list where every visible agent was busy.
func attentionScopeBoard() (model, api.BoardState) {
	m := newModel(nil, nil, "/r/sindri")
	m.scopeRepo = true
	b := api.BoardState{
		Projects: []api.Project{
			{Tag: "sin", Path: "/r/sindri"},
			{Tag: "oth", Path: "/r/other"},
		},
		Agents: []api.AgentView{
			{Name: "eitri", Project: "sin", Repo: "sindri", Status: "working"},
			{Name: "gloin", Project: "oth", Repo: "other", Status: "working"},
			{Name: "thrain", Project: "oth", Repo: "other", Status: api.StatusStalled},
		},
	}
	return m, b
}

// TestRepoScopeStillShowsAgentsWaitingOnYou: the attention marker on the handle counts the whole
// fleet on purpose, so a repo scope that hid the row it points at told the user something needed
// them and then showed them a list where nothing did. The stuck agent comes through the scope; the
// merely busy one from the same foreign repo does not, or the toggle would mean nothing.
func TestRepoScopeStillShowsAgentsWaitingOnYou(t *testing.T) {
	m, b := attentionScopeBoard()
	m.state = b

	var ids []string
	for _, r := range m.agentRows() {
		ids = append(ids, r.id)
	}
	joined := strings.Join(ids, " ")
	if !strings.Contains(joined, "thrain") {
		t.Errorf("a stalled agent in another repo must still be listed under repo scope, got %q", joined)
	}
	if strings.Contains(joined, "gloin") {
		t.Errorf("a busy agent in another repo must stay out of repo scope, got %q", joined)
	}
	if !strings.Contains(joined, "eitri") {
		t.Errorf("the local agent must still be listed, got %q", joined)
	}
}

// TestTheMarkedCountIsReachableInEveryScope is the invariant behind the fix: whatever the scope,
// every agent the handle's marker counts has a row to select — otherwise the marker names work the
// user cannot reach from where they are standing.
func TestTheMarkedCountIsReachableInEveryScope(t *testing.T) {
	m, b := attentionScopeBoard()
	m.state = b

	for _, scoped := range []bool{true, false} {
		m.scopeRepo = scoped
		listed := map[string]bool{}
		for _, r := range m.agentRows() {
			listed[r.id] = true
		}
		for _, a := range m.state.Agents {
			if api.AgentNeedsUser(a) && !listed[a.Name] {
				t.Errorf("scopeRepo=%v: %s (%s) is counted by the marker but has no row", scoped, a.Name, a.Status)
			}
		}
	}
}

// TestTheStuckRowCarriesTheWarningGlyph: the status word is one of four and sits in a column the
// eye skims, so the row that the handle's marker counts wears the same warning the CLI prints —
// what the user has to find is the row, not the vocabulary.
func TestTheStuckRowCarriesTheWarningGlyph(t *testing.T) {
	m, b := attentionScopeBoard()
	m.state = b

	for _, r := range m.agentRows() {
		marked := strings.Contains(r.text, "needs you")
		switch r.id {
		case "thrain":
			if !marked {
				t.Errorf("a stalled agent's row must say it needs you: %q", r.text)
			}
			if !strings.Contains(r.text, warnGlyph) {
				t.Errorf("the marker must carry the warning glyph: %q", r.text)
			}
		case "eitri":
			if marked {
				t.Errorf("a working agent's row must stay unmarked: %q", r.text)
			}
		}
	}
}

// TestTheBadgeStillMatchesTheRowsItSitsOver: admitting foreign stuck agents to the list moves the
// badge with it, or the header contradicts the list right below it — the rule tabCount exists for.
func TestTheBadgeStillMatchesTheRowsItSitsOver(t *testing.T) {
	m, b := attentionScopeBoard()
	m.state = b
	m.scopeRepo = true

	var agents tuiSection
	for _, s := range tuiSections {
		if s.Key == "agents" {
			agents = s
		}
	}
	if got, rows := m.tabCount(agents), itemRows(m.agentRows()); got != rows {
		t.Errorf("repo-scoped Agents badge = %d but the tab renders %d agent rows", got, rows)
	}
}
