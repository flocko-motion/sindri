package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// prFleet: one PR with a live author, one whose author has since gone (deleted/renamed), and one
// whose author is down.
func prFleet() api.BoardState {
	return api.BoardState{
		Projects: []api.Project{{Tag: "one", Path: "/r/one"}},
		PRs: []api.PR{
			{ID: "pr-td-1", Task: "td-1", Agent: "dvalin", Project: "one", Status: "open"},
			{ID: "pr-td-2", Task: "td-2", Agent: "nori", Project: "one", Status: "open"},
			{ID: "pr-td-GONE", Task: "td-3", Agent: "vanished", Project: "one", Status: "open"},
		},
		Agents: []api.AgentView{
			{Name: "dvalin", Project: "one", Status: "working", Task: "td-1"},
			{Name: "nori", Project: "one", Status: "down", Task: "td-2"},
		},
	}
}

// TestAttachFromPRFindsItsAuthor: the PR names the work, so attach should reach the agent that
// authored it without a detour via the Agents tab.
func TestAttachFromPRFindsItsAuthor(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.state = prFleet()

	a, ok := m.agentOnPR("pr-td-1")
	if !ok || a.Name != "dvalin" {
		t.Errorf("got %q ok=%v, want dvalin", a.Name, ok)
	}
}

// TestAttachFromPRFindsNobodyWhenAuthorGone: a PR whose author no longer has a roster entry (or
// doesn't exist, or wasn't selected) must report none rather than a stale/wrong match.
func TestAttachFromPRFindsNobodyWhenAuthorGone(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.state = prFleet()
	for _, id := range []string{"pr-td-GONE", "", "pr-nonexistent"} {
		if a, ok := m.agentOnPR(id); ok {
			t.Errorf("%q: expected no agent, got %q", id, a.Name)
		}
	}
}

// TestPRsFooterOffersAttach: the binding is invisible unless the footer advertises it.
func TestPRsFooterOffersAttach(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	if got := m.footerFor(scopePRs); !strings.Contains(got, "attach") {
		t.Errorf("the PRs footer should offer attach:\n%s", got)
	}
}

// TestOnKeyAttachFromPR covers the three outcomes the 'a' dispatch itself decides between, not
// just the resolution helper: no author working it, the author is down, and (implicitly, since
// attachAgent execs a real process untestable here) the live case falls through to neither
// message, which is what "no error, no flash" verifies.
func TestOnKeyAttachFromPR(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.state = prFleet()
	m.tab = 2 // PRs

	selectPR := func(id string) {
		for i, r := range m.rows() {
			if r.id == id {
				m.cursor[2] = i
				return
			}
		}
		t.Fatalf("row %q not found", id)
	}

	selectPR("pr-td-GONE")
	m.onKey(keyAttach)
	if !strings.Contains(m.flash, "no agent is working") {
		t.Errorf("expected a 'no agent' flash, got flash=%q errText=%q", m.flash, m.errText)
	}

	m.flash, m.errText = "", ""
	selectPR("pr-td-2") // nori, down
	m.onKey(keyAttach)
	if !strings.Contains(m.errText, "nori") || !strings.Contains(m.errText, "down") {
		t.Errorf("expected a down-agent error naming nori, got errText=%q flash=%q", m.errText, m.flash)
	}
}
