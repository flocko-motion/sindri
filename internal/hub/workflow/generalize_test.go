package workflow

import (
	"strings"
	"testing"
)

// TestTheFirstRejectionCarriesNoLecture: one round is the review doing its job — 62 of the fleet's
// PRs came back exactly once — and advice there is noise on the healthy case.
func TestTheFirstRejectionCarriesNoLecture(t *testing.T) {
	if got := GeneralizeNote(1); got != "" {
		t.Errorf("the first rejection said %q; one round is the system working", got)
	}
}

// TestTheNoteEscalatesWithTheCount: pr-sd-a6e884 took nine rounds, each opening "the previous
// findings are genuinely fixed" before naming a fresh instance of the same class. A note that reads
// the same at round 2 and round 9 is one the author has already learned to skim.
func TestTheNoteEscalatesWithTheCount(t *testing.T) {
	second, deep := GeneralizeNote(2), GeneralizeNote(4)
	if second == "" {
		t.Fatal("a second rejection said nothing — the count is the signal")
	}
	if !strings.Contains(second, "2") || !strings.Contains(deep, "4") {
		t.Error("the note must name the round; a worker told 'this is the 4th' checks its own work")
	}
	// The professionalism paragraph is the escalation, and it must not fire at the first repeat.
	if strings.Contains(second, "professionalism") {
		t.Error("round 2 already lectures on professionalism; it is a reminder, not yet a rebuke")
	}
	if !strings.Contains(deep, "professionalism") {
		t.Error("round 4 reads like round 2 — using review as a debugging loop must be named by then")
	}
	if len(deep) <= len(second) {
		t.Error("the deeper note is no stronger than the earlier one")
	}
}

// TestTheNoteAsksForTheClassAndTheSpec: the two things bombur's nine rounds show were skipped —
// checking the task against the diff, and fixing every instance rather than the case named.
func TestTheNoteAsksForTheClassAndTheSpec(t *testing.T) {
	note := GeneralizeNote(2)
	for _, want := range []string{"CLASS", "spec", "cross-check"} {
		if !strings.Contains(note, want) {
			t.Errorf("the note never mentions %q: %q", want, note)
		}
	}
}
