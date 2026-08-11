package theme

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestScopeNoteSaysWhichEffectApplies is the honesty both front-ends borrow from here: under an open
// parent the children are worked as one package, so carrying a rating to them orders them and releases
// nothing. The words for the two cases must not be interchangeable.
func TestScopeNoteSaysWhichEffectApplies(t *testing.T) {
	pkg := PriorityScopeNote(api.PriorityCascade{Children: 3, Unrated: 2})
	if !strings.Contains(pkg, "ORDER") || strings.Contains(pkg, "claimable") {
		t.Errorf("an open package's note must speak of order, not claimability: %q", pkg)
	}
	alone := PriorityScopeNote(api.PriorityCascade{Children: 3, Unrated: 2, Independent: true})
	if !strings.Contains(alone, "claimable") || strings.Contains(alone, "ORDER") {
		t.Errorf("children claimed on their own ARE released by a rating: %q", alone)
	}
	if got := PriorityScopeNote(api.PriorityCascade{}); got != "" {
		t.Errorf("nothing below means nothing to say, got %q", got)
	}
}

// TestScopeNoteReadsForOneTask: the note is a sentence a user reads, and "1 task below will be claimed
// on its own" is where a count-agnostic template starts writing nonsense.
func TestScopeNoteReadsForOneTask(t *testing.T) {
	for _, c := range []api.PriorityCascade{
		{Children: 1, Unrated: 1},
		{Children: 1, Unrated: 1, Independent: true},
	} {
		got := PriorityScopeNote(c)
		if strings.Contains(got, "1 tasks") || strings.Contains(got, "tasks below will") {
			t.Errorf("plural wording over a single task: %q", got)
		}
		if strings.Contains(got, "on its own — a priority is what makes each") {
			t.Errorf("mismatched number within the sentence: %q", got)
		}
	}
}

// TestScopeLabelsCarryTheRealCounts: a menu that named categories instead of numbers would leave the
// user to guess how much "all children" is.
func TestScopeLabelsCarryTheRealCounts(t *testing.T) {
	got := PriorityScopeLabels(api.PriorityCascade{Children: 3, Unrated: 2})
	if len(got) != len(api.PriorityScopes) {
		t.Fatalf("labels = %v, want one per scope", got)
	}
	if !strings.Contains(got[1], "2 tasks") {
		t.Errorf("the unrated scope must name how many carry nothing: %q", got[1])
	}
	if !strings.Contains(got[2], "3 tasks") {
		t.Errorf("the widest scope must name how many it overwrites: %q", got[2])
	}
}
