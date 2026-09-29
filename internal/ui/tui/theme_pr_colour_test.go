package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/api"
)

// sameStyle compares two styles by the only thing these tests care about — the colour they paint.
// lipgloss.Style holds a func, so it is not comparable directly.
func sameStyle(a, b lipgloss.Style) bool { return a.GetForeground() == b.GetForeground() }

// prBoard is a fleet with one live reviewer in repo "a" and none in repo "b" — the two sides of the
// one PR colour that depends on the fleet rather than the row.
func prBoard() []api.AgentView {
	return []api.AgentView{
		{Project: "a", Name: "fili", Role: "reviewer", Status: "idle"},
		{Project: "b", Name: "bombur", Role: "worker", Status: "working"},
	}
}

// TestRedIsExactlyWhatTheBadgeCounts is the invariant the whole colour scheme rests on: the badge
// counts the rows waiting on the user and red says this row is one of them, so a red row that is
// not counted (or a counted row that is not red) means one of the two is lying. Derived from one
// predicate, checked here across every state a PR can be in.
func TestRedIsExactlyWhatTheBadgeCounts(t *testing.T) {
	agents := prBoard()
	prs := []api.PR{
		{Project: "a", ID: "pr-open-reviewed", Status: "open"},
		{Project: "b", ID: "pr-open-stranded", Status: "open"},
		{Project: "a", ID: "pr-approved", Status: "approved"},
		{Project: "a", ID: "pr-rejected", Status: "rejected"},
		{Project: "a", ID: "pr-merged", Status: "merged"},
		{Project: "a", ID: "pr-scrapped", Status: "scrapped"},
		{Project: "a", ID: "pr-merging", Status: "merging"},
		{Project: "a", ID: "pr-merge-failed", Status: "merge-failed"},
		{Project: "a", ID: "pr-interim", Status: "open", Kind: "interim"},
	}
	for _, p := range prs {
		red := sameStyle(prStatusStyle(p, agents, false), stCrit)
		counted := api.PRNeedsUser(p, agents)
		if red != counted {
			t.Errorf("%s (%s): red=%v but the badge counts it=%v — one of them is lying to the user",
				p.ID, p.Status, red, counted)
		}
	}
}

// TestPRColoursSayWhatToDo pins the rest of the mapping, which is the cross-tab vocabulary: grey
// finished, orange mid-merge, cyan for the worker's rework, green for a review that is coming.
func TestPRColoursSayWhatToDo(t *testing.T) {
	agents := prBoard()
	cases := []struct {
		pr   api.PR
		want lipgloss.Style
		why  string
	}{
		{api.PR{Project: "a", Status: "merged"}, stDone, "finished, nothing to do"},
		{api.PR{Project: "a", Status: "scrapped"}, stDone, "finished, nothing to do"},
		{api.PR{Project: "a", Status: "merging"}, stTrans, "transitioning, as an agent launching is"},
		{api.PR{Project: "a", Status: "rejected"}, stWorking, "the worker's move, not yours"},
		{api.PR{Project: "a", Status: "open"}, stOpen, "a reviewer is alive in its repo: proceeding"},
	}
	for _, c := range cases {
		if got := prStatusStyle(c.pr, agents, false); !sameStyle(got, c.want) {
			t.Errorf("%s should read as %q", c.pr.Status, c.why)
		}
	}
	// The user's merge in flight reads as transitioning even before the hub confirms it.
	if got := prStatusStyle(api.PR{Project: "a", Status: "approved"}, agents, true); !sameStyle(got, stTrans) {
		t.Error("a merge the user just triggered is transitioning, not still waiting on them")
	}
}
