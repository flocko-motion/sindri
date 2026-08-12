package task

import (
	"strings"
	"testing"
)

// TestMintedIDsAreOwnedAndWellShaped: a new id must be recognised as sindri's by the same package
// that minted it — the drift this file exists to end was exactly a mint and a check disagreeing.
func TestMintedIDsAreOwnedAndWellShaped(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id, err := MintID()
		if err != nil {
			t.Fatalf("mint: %v", err)
		}
		if !IsOwned(id) {
			t.Fatalf("just-minted %q is not recognised as owned", id)
		}
		if OwnerOf(id) != OwnerSindri {
			t.Fatalf("just-minted %q routes to %v", id, OwnerOf(id))
		}
		// Six hex characters after the prefix, the shape td used and openspec still uses — it
		// travels into a PR id, a branch name and a worktree path.
		rest := id[len("sd-"):]
		if len(rest) != 6 || strings.ContainsFunc(rest, func(r rune) bool {
			return !strings.ContainsRune("0123456789abcdef", r)
		}) {
			t.Fatalf("%q is not prefix + 6 hex", id)
		}
		seen[id] = true
	}
	if len(seen) < 45 { // random, so not a strict guarantee — but a constant id would show here
		t.Errorf("only %d distinct ids in 50 mints", len(seen))
	}
}

// TestNewTasksAreMintedSd is the visible half of the change, spelled out rather than read back from
// the constant so that flipping the constant cannot silently flip the intent.
func TestNewTasksAreMintedSd(t *testing.T) {
	id, err := MintID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "sd-") {
		t.Errorf("new ids should be minted sd-, got %q", id)
	}
}

// TestLegacyIDsKeepWorking is the no-rewrite guarantee. An existing id is embedded in its PR id, its
// git branch name and its agent's worktree path, so it is never rewritten — which only works if the
// old prefix still reads as sindri's.
func TestLegacyIDsKeepWorking(t *testing.T) {
	const legacy = "td-abc123"
	if !IsOwned(legacy) {
		t.Error("a legacy td- id stopped being recognised as sindri's — its task could no longer be closed, prioritised or merged")
	}
	if OwnerOf(legacy) != OwnerSindri {
		t.Errorf("legacy id routes to %v, not sindri", OwnerOf(legacy))
	}
}

// TestOwnerOfRoutesEverySource: one question, one answer, for every prefix in the scheme.
func TestOwnerOfRoutesEverySource(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want Owner
	}{
		{"sd-abc123", OwnerSindri},
		{"td-abc123", OwnerSindri},
		{"os-abc123", OwnerOpenSpec},
		{"gh-42", OwnerGitHub},
		{"nonsense", OwnerUnknown},
		{"", OwnerUnknown},
	} {
		if got := OwnerOf(tc.id); got != tc.want {
			t.Errorf("OwnerOf(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

// TestOwnedAndLegacyTDAreDifferentQuestions is the trap the resolver exists to make unmissable.
// The td import filters rows coming OUT of a td database, so it asks "is this a td id" — if it
// asked "what do we mint" instead, the one code path whose purpose is carrying a td backlog across
// would silently import nothing now that the mint is sd-.
func TestOwnedAndLegacyTDAreDifferentQuestions(t *testing.T) {
	minted, err := MintID()
	if err != nil {
		t.Fatal(err)
	}
	if !IsOwned(minted) {
		t.Fatalf("%q should be owned", minted)
	}
	if IsLegacyTD(minted) {
		t.Errorf("a freshly minted id %q must not read as a legacy td id", minted)
	}
	// And the converse: a td id is both, which is why the two predicates cannot be told apart by
	// example alone — only by asking the right one.
	if !IsLegacyTD("td-abc123") || !IsOwned("td-abc123") {
		t.Error("a td- id must satisfy both questions")
	}
	if IsLegacyTD("os-abc123") || IsLegacyTD("gh-1") {
		t.Error("a mirrored id is not a legacy td id")
	}
}

// TestIsIDClaimsOnlyAWholeID is what lets an id be OPTIONAL in front of free text: `comment` reads
// its first argument as the task to comment on only if that argument IS one, so the check has to
// refuse a sentence that merely opens with a prefix. OwnerOf alone would claim every line below.
func TestIsIDClaimsOnlyAWholeID(t *testing.T) {
	for _, id := range []string{"sd-abc123", "td-abc123", "os-abc123", "gh-42", "sd-a_b-c"} {
		if !IsID(id) {
			t.Errorf("IsID(%q) = false, want true", id)
		}
	}
	for _, prose := range []string{
		"sd-1c3041 is the task this corrects", // the case that made the id look mandatory
		"td- ",
		"sd-",
		"the body is stale",
		"gh-42, and the one before it",
		"",
	} {
		if IsID(prose) {
			t.Errorf("IsID(%q) = true — prose would be eaten as the comment's target", prose)
		}
	}
}

// TestGitHubIDRoundTrips: the adapter builds and reverses issue ids through here, so the pair must
// agree, and must refuse an id belonging to another source rather than returning a plausible zero.
func TestGitHubIDRoundTrips(t *testing.T) {
	id := GitHubID(42)
	if n, ok := GitHubNumber(id); !ok || n != 42 {
		t.Errorf("GitHubNumber(%q) = %d, %v", id, n, ok)
	}
	for _, foreign := range []string{"sd-abc123", "td-abc123", "os-abc123", "gh-notanumber"} {
		if _, ok := GitHubNumber(foreign); ok {
			t.Errorf("GitHubNumber(%q) claimed it", foreign)
		}
	}
}

// TestSpecIDIsRecognisedAsOpenSpec closes the loop for the one source that mints from a hash.
func TestSpecIDIsRecognisedAsOpenSpec(t *testing.T) {
	id := SpecID("abc123")
	if OwnerOf(id) != OwnerOpenSpec {
		t.Errorf("SpecID produced %q, which routes to %v", id, OwnerOf(id))
	}
	if IsOwned(id) {
		t.Errorf("%q is mirrored, not sindri's own", id)
	}
}
