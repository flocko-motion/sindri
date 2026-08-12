package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestTheGateCountRidesOnTheTasksTab: work awaiting a verdict is invisible to every worker, so a
// backlog of it reads as plenty to do beside an idle agent. The count sits in the header, which is
// on screen from whichever tab you are looking at — the question it answers ("why is nothing
// happening?") is usually asked from the Agents tab.
func TestTheGateCountRidesOnTheTasksTab(t *testing.T) {
	m := newModel(nil, nil, "")
	m.w, m.h = 120, 40
	m.state = api.BoardState{Tasks: []api.Task{
		{ID: "td-1", Status: "open", Approval: "pending"},
		{ID: "td-2", Status: "open", Approval: "pending"},
		{ID: "td-3", Status: "open"},
		{ID: "td-4", Status: "closed", Approval: "pending"}, // ended: it decides nothing
	}}
	m.reclamp()

	view := m.View()
	if !strings.Contains(view, "2"+gateGlyph) {
		t.Errorf("the header should show the two tasks awaiting a verdict:\n%s", firstLine(view))
	}

	// With nothing gated the marker is absent — it is a call to act, not furniture.
	m.state = api.BoardState{Tasks: []api.Task{{ID: "td-3", Status: "open"}}}
	if view := m.View(); strings.Contains(view, gateGlyph) {
		t.Errorf("no gated work should mean no marker:\n%s", firstLine(view))
	}
}

// TestCountAwaitingVerdictCountsOnlyLiveProposals: a rejected task has had its verdict, and one that
// has ended decides nothing — neither is waiting on anybody.
func TestCountAwaitingVerdictCountsOnlyLiveProposals(t *testing.T) {
	got := api.CountAwaitingVerdict([]api.Task{
		{ID: "a", Status: "open", Approval: "pending"},
		{ID: "b", Status: "open", Approval: "rejected"},
		{ID: "c", Status: "open", Approval: "approved"},
		{ID: "d", Status: "closed", Approval: "pending"},
		{ID: "e", Status: "open"},
	})
	if got != 1 {
		t.Errorf("CountAwaitingVerdict = %d, want 1 (only the live proposal)", got)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
