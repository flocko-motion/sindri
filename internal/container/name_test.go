package container

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestRepoSlugKeepsUnderscoreButStripsSigils pins why the reviewer pool's virtual project is spelled
// "_global" rather than "$global" or "*global": repoSlug's charset keeps underscore but drops both
// sigils, so either one collides with a directory literally named "global" while "_global" stays
// distinct.
func TestRepoSlugKeepsUnderscoreButStripsSigils(t *testing.T) {
	for _, tc := range []struct{ dir, want string }{
		{"global", "global"},
		{"$global", "global"},
		{"*global", "global"},
		{"_global", "_global"},
	} {
		if got := repoSlug("/repos/" + tc.dir); got != tc.want {
			t.Errorf("repoSlug(%q) = %q, want %q", tc.dir, got, tc.want)
		}
	}
}

// TestRepoTagNeverProducesGlobal pins the other half: RepoTag is lowercase hex, an alphabet that
// excludes g, l and o — so no repo's hash-derived tag can ever equal the literal string "global" and
// collide with the virtual project.
func TestRepoTagNeverProducesGlobal(t *testing.T) {
	const hex = "0123456789abcdef"
	tag := api.RepoTag("/some/repo")
	for _, r := range tag {
		if !strings.ContainsRune(hex, r) {
			t.Fatalf("RepoTag produced non-hex rune %q in %q", r, tag)
		}
	}
	for _, r := range "global" {
		if !strings.ContainsRune(hex, r) {
			return // confirmed: "global" contains a rune outside RepoTag's alphabet
		}
	}
	t.Fatal(`"global" is entirely hex digits — RepoTag could collide with it`)
}
