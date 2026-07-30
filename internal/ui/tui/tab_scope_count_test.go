package tui

import (
	"fmt"
	"testing"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

// scopeBoard: two agents and one open PR in the selected repo ("sin"), the rest of the
// fleet elsewhere — 17 agents and 3 open PRs in total. One merged PR in the selected
// repo checks that the scoped PR badge still honours open-ness, not just the repo tag.
func scopeBoard() (model, hub.BoardState) {
	m := newModel(nil, nil, "/r/sindri")
	b := hub.BoardState{
		Projects: []store.Project{
			{Tag: "sin", Path: "/r/sindri"},
			{Tag: "oth", Path: "/r/other"},
		},
		PRs: []store.PR{
			{ID: "pr-1", Project: "sin", Status: "open"},
			{ID: "pr-2", Project: "sin", Status: "merged"}, // in scope but terminal
			{ID: "pr-3", Project: "oth", Status: "open"},
			{ID: "pr-4", Project: "oth", Status: "open"},
		},
	}
	b.Agents = append(b.Agents,
		hub.AgentView{Name: "eitri", Project: "sin", Status: "working"},
		hub.AgentView{Name: "dvalin", Project: "sin", Status: "down"}, // down agents still count
	)
	for i := 0; i < 15; i++ {
		b.Agents = append(b.Agents, hub.AgentView{Name: fmt.Sprintf("far-%d", i), Project: "oth", Status: "working"})
	}
	return m, b
}

// TestTabCountFollowsScope: the § toggle re-scopes the Agents/PRs lists, so their tab
// badges have to move with it — 2 local agents must read "2" even though the fleet has
// 17, otherwise the header contradicts the list right below it.
func TestTabCountFollowsScope(t *testing.T) {
	m, b := scopeBoard()
	m.state = b

	section := func(key string) hub.Section {
		for _, s := range hub.Sections {
			if s.Key == key {
				return s
			}
		}
		t.Fatalf("no %q section", key)
		return hub.Section{}
	}
	agents, prs := section("agents"), section("prs")

	m.scopeRepo = true
	if got := m.tabCount(agents); got != 2 {
		t.Errorf("repo-scoped Agents badge = %d, want 2", got)
	}
	if got := m.tabCount(prs); got != 1 {
		t.Errorf("repo-scoped PRs badge = %d, want 1 (open, this repo)", got)
	}

	m.scopeRepo = false
	if got := m.tabCount(agents); got != 17 {
		t.Errorf("global Agents badge = %d, want 17", got)
	}
	if got := m.tabCount(prs); got != 3 {
		t.Errorf("global PRs badge = %d, want 3 (open, fleet-wide)", got)
	}

	// Global scope must reproduce the registry's fleet-wide counts exactly — tabCount
	// is not allowed to invent a second definition of "how many".
	if got, want := m.tabCount(agents), agents.Count(m.state); got != want {
		t.Errorf("global Agents badge = %d, registry says %d", got, want)
	}
	if got, want := m.tabCount(prs), prs.Count(m.state); got != want {
		t.Errorf("global PRs badge = %d, registry says %d", got, want)
	}
}

// TestTabCountMatchesRows is the invariant that motivates the shared inScope predicate:
// whatever the scope, each badge equals the number of rows the tab renders. Agents is
// compared against the roster rows only (agentRows appends orphan warnings, which are
// containers with no agent and deliberately outside the roster count). PRs holds at the
// default f-filter, whose "unmerged" rule is the same as PROpen; the badge tracks scope,
// not the f-toggle, so showing merged PRs is expected to exceed the open count.
func TestTabCountMatchesRows(t *testing.T) {
	m, b := scopeBoard()
	m.state = b

	for _, scoped := range []bool{true, false} {
		m.scopeRepo = scoped
		for _, s := range hub.Sections {
			var rows int
			switch s.Key {
			case "agents":
				rows = len(m.agentRows()) - len(m.state.Orphans)
			case "prs":
				rows = len(m.prRows())
			default:
				continue
			}
			if got := m.tabCount(s); got != rows {
				t.Errorf("scopeRepo=%v: %s badge = %d but the tab renders %d rows", scoped, s.Key, got, rows)
			}
		}
	}
}

// TestTabCountScopeInvariantSections: Tasks is always the selected repo's and Repos /
// Meeting are global by nature, so the § toggle must not move their badges.
func TestTabCountScopeInvariantSections(t *testing.T) {
	m, b := scopeBoard()
	b.Tasks = []store.Task{{ID: "a", Status: "open"}, {ID: "b", Status: "closed"}}
	m.state = b

	for _, s := range hub.Sections {
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
