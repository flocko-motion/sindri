package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestRepoScopeKeepsAPRWaitingOnYou is the asymmetry this closes. Agents already crossed repos when
// they were stuck on the user; PRs did not, so an approved PR in another repo — the one action only
// the user can take — was invisible until they switched to that repo, and nothing told them to.
// Background work is exactly the work that must alert wherever attention happens to be.
func TestRepoScopeKeepsAPRWaitingOnYou(t *testing.T) {
	m, b := scopeBoard()
	m.state = b
	m.scopeRepo = true

	rows := m.prRows()
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.id)
	}
	got := strings.Join(ids, " ")
	if !strings.Contains(got, "pr-4") {
		t.Errorf("an approved PR elsewhere waits on the user and must show in repo scope, got %s", got)
	}
	if !strings.Contains(got, "pr-1") {
		t.Errorf("the active repo's own PRs must still show, got %s", got)
	}
	// And the scope still means something: a foreign PR with a reviewer of its own asks nothing of
	// the user, so it stays out. Without this the toggle would be doing no work at all.
	if strings.Contains(got, "pr-3") {
		t.Errorf("a foreign PR nobody is waiting on must stay out of repo scope, got %s", got)
	}
	// The badge is the same claim rendered twice, so it counts exactly what the list shows — the
	// selectable rows, since a grouped list also carries the headings that label them.
	for _, s := range tuiSections {
		if s.Key == "prs" && m.tabCount(s) != itemRows(rows) {
			t.Errorf("PRs badge = %d but the tab renders %d PR rows", m.tabCount(s), itemRows(rows))
		}
	}
}

// TestForeignPRsAreGroupedByRepo: a row from elsewhere interleaved by age reads as a filter that
// has stopped working. Gathered under its own repo it reads as what it is — the same thing the
// repo key does for a stuck foreign agent.
func TestForeignPRsAreGroupedByRepo(t *testing.T) {
	m, b := scopeBoard()
	// Two more locals, so an age-ordered list would put the foreign row in the middle.
	b.PRs = append(b.PRs,
		api.PR{ID: "pr-5", Project: "sin", Status: "open"},
		api.PR{ID: "pr-6", Project: "sin", Status: "open"},
	)
	m.state = b
	m.scopeRepo = true

	var repos []string
	for _, r := range m.prRows() {
		for _, p := range m.state.PRs {
			if p.ID == r.id {
				repos = append(repos, p.Project)
			}
		}
	}
	// Every repo appears in one run: seeing a repo again after leaving it is the interleaving.
	seen := map[string]bool{}
	for i, repo := range repos {
		if i > 0 && repos[i-1] == repo {
			continue
		}
		if seen[repo] {
			t.Fatalf("rows are interleaved by repo, not grouped: %v", repos)
		}
		seen[repo] = true
	}
}

// TestScopeLabelSaysWhatItDoes: the toggle admits foreign rows that need the user, so a label
// reading "repo" would be a filter claiming to exclude what it shows.
func TestScopeLabelSaysWhatItDoes(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab = 2 // PRs: scopeNeedsYou(scopePRs) is true
	if got := scopeName(true, m); got == "repo" {
		t.Errorf("the narrow scope is not the repo alone — %q claims it is", got)
	}
	if !strings.Contains(scopeName(true, m), "repo") {
		t.Errorf("it is still repo-first, and the label should say so: %q", scopeName(true, m))
	}
	if got := scopeName(false, m); got != "global" {
		t.Errorf("the wide scope is unchanged, got %q", got)
	}
}
