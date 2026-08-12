package api

import "testing"

// names flattens a sorted roster to its agent names, for a compact want-comparison.
func names(agents []AgentView) []string {
	out := make([]string, len(agents))
	for i, a := range agents {
		out[i] = a.Name
	}
	return out
}

// TestSortedAgentsOrdersByRepoThenRoleThenName is the base case: two repos, mixed roles within
// each, and the deliberate role order (worker, reviewer, planner, coauthor) rather than alphabetical.
func TestSortedAgentsOrdersByRepoThenRoleThenName(t *testing.T) {
	projects := []Project{
		{Tag: "b", Path: "/repos/beta"},
		{Tag: "a", Path: "/repos/alpha"},
	}
	agents := []AgentView{
		{Project: "b", Name: "zed", Role: "worker"},
		{Project: "a", Name: "nori", Role: "coauthor"},
		{Project: "a", Name: "dvalin", Role: "worker"},
		{Project: "a", Name: "brokkr", Role: "reviewer"},
		{Project: "a", Name: "galar", Role: "planner"},
	}
	got := names(SortedAgents(agents, projects))
	want := []string{"dvalin", "brokkr", "galar", "nori", "zed"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: got %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestSortedAgentsMatchesTheRepoListOrder: repos group in path order — the same order Projects()
// (and so the Repos list) is in — not the order the caller happened to list projects.
func TestSortedAgentsMatchesTheRepoListOrder(t *testing.T) {
	projects := []Project{
		{Tag: "z", Path: "/z-repo"},
		{Tag: "a", Path: "/a-repo"},
	}
	agents := []AgentView{
		{Project: "z", Name: "first-listed", Role: "worker"},
		{Project: "a", Name: "path-first", Role: "worker"},
	}
	got := names(SortedAgents(agents, projects))
	if got[0] != "path-first" {
		t.Errorf("got %v, want the /a-repo agent first (path order), not project-list order", got)
	}
}

// TestSortedAgentsIsStable: agents tying on repo, role and name (same agent, read twice, or two
// distinct rows an upstream bug left identical) must not reorder between calls — a shuffling cursor
// is worse than an arbitrary but fixed tie-break.
func TestSortedAgentsIsStable(t *testing.T) {
	projects := []Project{{Tag: "a", Path: "/a"}}
	agents := []AgentView{
		{Project: "a", Name: "dvalin", Role: "worker", Task: "first"},
		{Project: "a", Name: "dvalin", Role: "worker", Task: "second"},
	}
	got := SortedAgents(agents, projects)
	if got[0].Task != "first" || got[1].Task != "second" {
		t.Errorf("a stable sort must keep tying rows in their original order, got %+v", got)
	}
}

// TestSortedAgentsPutsAnUnregisteredRepoLast: an agent whose project isn't in the Projects list
// (a race with a forgotten repo, say) must not sort to the front just because "" < every path.
func TestSortedAgentsPutsAnUnregisteredRepoLast(t *testing.T) {
	projects := []Project{{Tag: "known", Path: "/z-repo"}} // path sorts AFTER nothing, on purpose
	agents := []AgentView{
		{Project: "unknown", Name: "orphaned", Role: "worker"},
		{Project: "known", Name: "registered", Role: "worker"},
	}
	got := names(SortedAgents(agents, projects))
	want := []string{"registered", "orphaned"}
	if got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want the unregistered project's agent last: %v", got, want)
	}
}

// TestSortedAgentsDoesNotMutateItsInput: the caller's slice (e.g. the model's own board state) must
// come back unchanged — SortedAgents hands back a new one, the way ArrangeTasks does.
func TestSortedAgentsDoesNotMutateItsInput(t *testing.T) {
	projects := []Project{{Tag: "a", Path: "/a"}}
	agents := []AgentView{
		{Project: "a", Name: "zed", Role: "worker"},
		{Project: "a", Name: "adam", Role: "worker"},
	}
	original := append([]AgentView(nil), agents...)
	_ = SortedAgents(agents, projects)
	for i := range agents {
		if agents[i] != original[i] {
			t.Errorf("input mutated at %d: got %+v, want %+v", i, agents[i], original[i])
		}
	}
}
