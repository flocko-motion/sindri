package api

import "testing"

// TestSortedPRsGroupsByRepoAndKeepsOrderWithin: grouping is what makes a foreign row legible when
// a scope keeps it for needing the user, and the order inside a repo is the store's — newest
// first — which a non-stable sort would scramble for no reason.
func TestSortedPRsGroupsByRepoAndKeepsOrderWithin(t *testing.T) {
	projects := []Project{{Tag: "b", Path: "/r/beta"}, {Tag: "a", Path: "/r/alpha"}}
	// Interleaved, as the store's fleet-wide newest-first query returns them.
	in := []PR{
		{ID: "b1", Project: "b"},
		{ID: "a1", Project: "a"},
		{ID: "b2", Project: "b"},
		{ID: "a2", Project: "a"},
	}
	got := SortedPRs(in, projects)
	var ids []string
	for _, p := range got {
		ids = append(ids, p.ID)
	}
	// By repo PATH, so the PRs list groups repos in the sequence the Repos list does: /r/alpha first.
	want := []string{"a1", "a2", "b1", "b2"}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("SortedPRs = %v, want %v", ids, want)
		}
	}
	// The input is not mutated: the caller's slice is the board's, shared with everything else
	// reading it.
	if in[0].ID != "b1" {
		t.Errorf("SortedPRs reordered its argument: %s", in[0].ID)
	}
}

// TestSortedPRsPutsAnUnknownRepoLast: a PR whose project the registry cannot name sorts last, not
// first — plain "" < x would head the list with the one repo the caller cannot label.
func TestSortedPRsPutsAnUnknownRepoLast(t *testing.T) {
	projects := []Project{{Tag: "a", Path: "/r/alpha"}}
	got := SortedPRs([]PR{{ID: "x", Project: "gone"}, {ID: "a1", Project: "a"}}, projects)
	if got[0].ID != "a1" || got[1].ID != "x" {
		t.Errorf("an unnameable repo should sort last, got %s then %s", got[0].ID, got[1].ID)
	}
}

// TestRepoNameFallsBackToTheTag: both front-ends label rows with this, and a row it could not name
// must still be placeable rather than blank.
func TestRepoNameFallsBackToTheTag(t *testing.T) {
	projects := []Project{{Tag: "a", Path: "/r/alpha"}}
	if got := RepoName(projects, "a"); got != "alpha" {
		t.Errorf("RepoName = %q, want alpha", got)
	}
	if got := RepoName(projects, "gone"); got != "gone" {
		t.Errorf("an unregistered tag should read as itself, got %q", got)
	}
}
