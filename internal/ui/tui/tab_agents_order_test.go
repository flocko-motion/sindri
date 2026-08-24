package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// rowIDsInOrder is the ids of the rows a list holds, in order — its labels and section headings
// dropped, since they select nothing and a question about order is a question about the rows.
func rowIDsInOrder(rows []row) []string {
	var out []string
	for _, r := range items(rows) {
		out = append(out, r.id)
	}
	return out
}

// TestAgentRowsOrderByRepoThenRoleThenName: the Agents tab must show the same order
// `sindri agent list` does (api.SortedAgents) — repo by path, then role, then name — not the raw
// board order (project hash, then name, role ignored).
func TestAgentRowsOrderByRepoThenRoleThenName(t *testing.T) {
	m := newModel(nil, nil, "")
	m.scopeRepo = false
	m.state = api.BoardState{
		Projects: []api.Project{
			{Tag: "b", Path: "/repos/beta"},
			{Tag: "a", Path: "/repos/alpha"},
		},
		Agents: []api.AgentView{
			{Project: "b", Name: "zed", Role: "worker"},
			{Project: "a", Name: "nori", Role: "coauthor"},
			{Project: "a", Name: "dvalin", Role: "worker"},
		},
	}
	got := rowIDsInOrder(m.agentRows())
	want := []string{"dvalin", "nori", "zed"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: got %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestAgentRowsInRepoScopeOrderByRoleThenName: with the scope toggle narrowed to one repo, every
// visible row shares that repo, so the repo key is a no-op tie and the effective order is
// role-then-name — not a redundant grouping level, and not the board's raw project-then-name order.
func TestAgentRowsInRepoScopeOrderByRoleThenName(t *testing.T) {
	m := newModel(nil, nil, "/repos/alpha")
	m.scopeRepo = true
	m.state = api.BoardState{
		Projects: []api.Project{{Tag: "a", Path: "/repos/alpha"}},
		Agents: []api.AgentView{
			{Project: "a", Name: "zed", Role: "worker"},
			{Project: "a", Name: "nori", Role: "coauthor"},
			{Project: "a", Name: "brokkr", Role: "reviewer"},
			{Project: "a", Name: "adam", Role: "worker"},
		},
	}
	got := rowIDsInOrder(m.agentRows())
	want := []string{"adam", "zed", "brokkr", "nori"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: got %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestAgentListCLIMatchesTheTUIOrder pins the interchangeable-front-ends rule directly: both call
// api.SortedAgents over the same board, so they cannot drift onto different orders.
func TestAgentListCLIMatchesTheTUIOrder(t *testing.T) {
	projects := []api.Project{
		{Tag: "b", Path: "/repos/beta"},
		{Tag: "a", Path: "/repos/alpha"},
	}
	agents := []api.AgentView{
		{Project: "b", Name: "zed", Role: "worker"},
		{Project: "a", Name: "nori", Role: "coauthor"},
		{Project: "a", Name: "dvalin", Role: "worker"},
	}
	m := newModel(nil, nil, "")
	m.scopeRepo = false
	m.state = api.BoardState{Projects: projects, Agents: agents}

	tuiOrder := rowIDsInOrder(m.agentRows())
	cliOrder := make([]string, 0, len(agents))
	for _, a := range api.SortedAgents(agents, projects) {
		cliOrder = append(cliOrder, a.Name)
	}
	if strings.Join(tuiOrder, ",") != strings.Join(cliOrder, ",") {
		t.Errorf("TUI order %v disagrees with the CLI's %v", tuiOrder, cliOrder)
	}
}
