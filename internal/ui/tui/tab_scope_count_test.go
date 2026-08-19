package tui

import (
	"fmt"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// scopeBoard: two agents and one open PR in the selected repo ("sin"), the rest of the
// fleet elsewhere — 17 agents and 3 open PRs in total. One merged PR in the selected
// repo checks that the scoped PR badge still honours open-ness, not just the repo tag.
//
// The foreign PRs are deliberately one of each kind, because repo scope is no longer the repo
// alone: pr-3 has a live reviewer in its own repo, so nothing is asked of the user and scope hides
// it; pr-4 is approved, which only the user can act on, so it crosses. One of the far agents is
// that reviewer rather than an eighteenth, so the agent counts stay what they were.
func scopeBoard() (model, api.BoardState) {
	m := newModel(nil, nil, "/r/sindri")
	b := api.BoardState{
		Projects: []api.Project{
			{Tag: "sin", Path: "/r/sindri"},
			{Tag: "oth", Path: "/r/other"},
		},
		PRs: []api.PR{
			{ID: "pr-1", Project: "sin", Status: "open"},
			{ID: "pr-2", Project: "sin", Status: "merged"}, // in scope but terminal
			{ID: "pr-3", Project: "oth", Status: "open"},
			{ID: "pr-4", Project: "oth", Status: "approved"}, // waits on the user's merge
		},
	}
	b.Agents = append(b.Agents,
		api.AgentView{Name: "eitri", Project: "sin", Status: "working"},
		api.AgentView{Name: "dvalin", Project: "sin", Status: "down"}, // down agents still count
	)
	for i := 0; i < 15; i++ {
		a := api.AgentView{Name: fmt.Sprintf("far-%d", i), Project: "oth", Status: "working"}
		if i == 0 {
			a.Role = "reviewer" // so pr-3 waits on a review that IS coming
		}
		b.Agents = append(b.Agents, a)
	}
	return m, b
}

// TestTabCountFollowsScope: the § toggle re-scopes the Agents/PRs lists, so their tab
// badges have to move with it — 2 local agents must read "2" even though the fleet has
// 17, otherwise the header contradicts the list right below it.
//
// "Re-scopes" is not "hides every other repo": both lists keep whatever waits on the user, wherever
// it is, because agents and PRs run while attention is elsewhere. So the scoped PR badge is the
// local open one PLUS the approved foreign one, and pr-3 — foreign with a reviewer of its own —
// is what proves the scope still bites.
func TestTabCountFollowsScope(t *testing.T) {
	m, b := scopeBoard()
	m.state = b

	section := func(key string) tuiSection {
		for _, s := range tuiSections {
			if s.Key == key {
				return s
			}
		}
		t.Fatalf("no %q section", key)
		return tuiSection{}
	}
	agents, prs := section("agents"), section("prs")

	m.scopeRepo = true
	if got := m.tabCount(agents); got != 2 {
		t.Errorf("repo-scoped Agents badge = %d, want 2", got)
	}
	if got := m.tabCount(prs); got != 2 {
		t.Errorf("repo-scoped PRs badge = %d, want 2 (this repo's open one, plus the foreign PR waiting on the user)", got)
	}

	m.scopeRepo = false
	if got := m.tabCount(agents); got != 17 {
		t.Errorf("global Agents badge = %d, want 17", got)
	}
	if got := m.tabCount(prs); got != 3 {
		t.Errorf("global PRs badge = %d, want 3 (open, fleet-wide)", got)
	}

	// Global scope must reproduce the board's own fleet-wide counts exactly — tabCount
	// is not allowed to invent a second definition of "how many".
	if got, want := m.tabCount(agents), m.state.AgentCount(); got != want {
		t.Errorf("global Agents badge = %d, board says %d", got, want)
	}
	if got, want := m.tabCount(prs), m.state.OpenPRCount(); got != want {
		t.Errorf("global PRs badge = %d, board says %d", got, want)
	}
}

// TestTabCountMatchesRows is the invariant that motivates the shared inScope predicate:
// whatever the scope, each badge equals the number of rows the tab renders. Counted over the
// selectable rows, since a scope holding foreign rows also carries the headings that label them.
// Agents is compared against the roster rows only (agentRows appends orphan warnings, which are
// containers with no agent and deliberately outside the roster count). PRs holds at the
// default f-filter, whose "unmerged" rule is the same as PROpen; the badge tracks scope,
// not the f-toggle, so showing merged PRs is expected to exceed the open count.
func TestTabCountMatchesRows(t *testing.T) {
	m, b := scopeBoard()
	m.state = b

	for _, scoped := range []bool{true, false} {
		m.scopeRepo = scoped
		for _, s := range tuiSections {
			var rows int
			switch s.Key {
			case "agents":
				rows = itemRows(m.agentRows()) - len(m.state.Orphans)
			case "prs":
				rows = itemRows(m.prRows())
			default:
				continue
			}
			if got := m.tabCount(s); got != rows {
				t.Errorf("scopeRepo=%v: %s badge = %d but the tab renders %d rows", scoped, s.Key, got, rows)
			}
		}
	}
}

// TestGlobalReviewerCountsInNarrowScope: a GlobalProject reviewer belongs to no repo, so the narrow
// scope must count it without needing api.AgentNeedsUser — unlike a genuinely foreign, idle agent,
// which the scope still excludes (scopeBoard's 15 far-N agents do not move the badge above).
func TestGlobalReviewerCountsInNarrowScope(t *testing.T) {
	m, b := scopeBoard()
	b.Agents = append(b.Agents, api.AgentView{Name: "ori", Project: api.GlobalProject, Role: "reviewer", Status: "idle"})
	m.state = b
	m.scopeRepo = true

	var agents tuiSection
	for _, s := range tuiSections {
		if s.Key == "agents" {
			agents = s
		}
	}
	if got := m.tabCount(agents); got != 3 {
		t.Errorf("repo-scoped Agents badge = %d, want 3 (2 local + the idle global reviewer)", got)
	}
	if !m.inScope(api.GlobalProject) {
		t.Error("GlobalProject should always be in scope — it belongs to no repo to be foreign to")
	}
}

// TestTabCountScopeInvariantSections: Tasks is always the selected repo's and Repos /
// Meeting are global by nature, so the § toggle must not move their badges.
func TestTabCountScopeInvariantSections(t *testing.T) {
	m, b := scopeBoard()
	b.Tasks = []api.Task{{ID: "a", Status: "open"}, {ID: "b", Status: "closed"}}
	m.state = b

	for _, s := range tuiSections {
		if s.Key == "agents" || s.Key == "prs" {
			continue
		}
		m.scopeRepo = true
		scoped := m.tabCount(s)
		m.scopeRepo = false
		if global := m.tabCount(s); scoped != global {
			t.Errorf("%s badge should not follow scope: repo=%d global=%d", s.Key, scoped, global)
		}
	}
}
