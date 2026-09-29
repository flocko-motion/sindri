package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestRepoDetailShowsTheReferenceBranch: the branch agents work against decided every claim and
// merge while being invisible here, so the user never learned the key existed.
func TestRepoDetailShowsTheReferenceBranch(t *testing.T) {
	pinned := repoDetail(t, api.RepoDocState{Reference: "trunk", ReferencePinned: true})
	if !strings.Contains(pinned, "trunk") {
		t.Errorf("a pinned reference should be named, got:\n%s", pinned)
	}
	if strings.Contains(pinned, warnGlyph) {
		t.Errorf("a pinned reference needs no warning, got:\n%s", pinned)
	}

	// Unpinned follows the main checkout. That is the design, so it is marked the way a defaulted
	// architecture doc is marked — and nothing here may read as a setting the user forgot.
	follows := repoDetail(t, api.RepoDocState{Reference: "master"})
	if !strings.Contains(follows, "master") || !strings.Contains(follows, "main checkout") {
		t.Errorf("an unpinned reference should be named and marked, got:\n%s", follows)
	}
	if strings.Contains(follows, warnGlyph) || strings.Contains(follows, "press E") {
		t.Errorf("following the checkout is no fault to fix, got:\n%s", follows)
	}
}

// TestRepoDetailWarnsWithNoBranchToWorkFrom: a detached main checkout leaves the project with no
// reference, which jams every claim — the one reference situation worth the gate's warning style.
func TestRepoDetailWarnsWithNoBranchToWorkFrom(t *testing.T) {
	got := repoDetail(t, api.RepoDocState{ReferenceAdvice: "… is on a detached HEAD, so there is no reference branch to work from"})
	if !strings.Contains(got, "no reference branch") || !strings.Contains(got, warnGlyph) {
		t.Errorf("a repo with nothing to work from must say so, got:\n%s", got)
	}
	if !strings.Contains(got, "check one out") {
		t.Errorf("the warning should name the fix — a branch to check out, got:\n%s", got)
	}

	// The other fault is a `reference:` naming a branch that isn't there; telling that user to check
	// out a branch would send them to fix the one thing that is fine.
	miss := repoDetail(t, api.RepoDocState{ReferencePinned: true, ReferenceAdvice: "the configured reference branch \"nope\" doesn't exist"})
	if !strings.Contains(miss, "reference") || strings.Contains(miss, "check one out") {
		t.Errorf("a missing pinned branch is a config fault, got:\n%s", miss)
	}
}

// TestRepoDetailSilentAboutReferenceWithoutSnapshot: an older hub sends no reference fields, and a
// warning invented from an absent one would call every repo broken.
func TestRepoDetailSilentAboutReferenceWithoutSnapshot(t *testing.T) {
	got := repoDetail(t, api.RepoDocState{Doc: "ARCHITECTURE.md", Readable: true, Set: true})
	if strings.Contains(got, "ref:") || strings.Contains(got, "reference") {
		t.Errorf("no snapshot should render no reference line, got:\n%s", got)
	}
}
