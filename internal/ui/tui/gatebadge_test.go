package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestTheAttentionMarkerRidesOnEveryHandle: what waits on the user is drawn beside the tab that
// holds it, from the counts the hub resolved. All three markers come out of one loop over the
// sections, which is the point — a fourth is a line in the hub's registry, not a case in this view.
func TestTheAttentionMarkerRidesOnEveryHandle(t *testing.T) {
	m := newModel(nil, nil, "")
	m.w, m.h = 120, 40
	m.state = api.BoardState{Sections: []api.Section{
		{Key: "tasks", Title: "Tasks", Count: 3, Attention: 2},
		{Key: "agents", Title: "Agents", Count: 5, Attention: 1},
		{Key: "prs", Title: "PRs", Count: 4, Attention: 3},
	}}
	m.reclamp()

	view := m.View()
	for _, want := range []string{"2" + attentionGlyph, "1" + attentionGlyph, "3" + attentionGlyph} {
		if !strings.Contains(view, want) {
			t.Errorf("the header should show %q:\n%s", want, firstLine(view))
		}
	}

	// With nothing waiting the marker is absent — it is a call to act, not furniture.
	m.state = api.BoardState{Sections: []api.Section{{Key: "tasks", Title: "Tasks", Count: 3}}}
	if view := m.View(); strings.Contains(view, attentionGlyph) {
		t.Errorf("nothing waiting on the user should mean no marker:\n%s", firstLine(view))
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
