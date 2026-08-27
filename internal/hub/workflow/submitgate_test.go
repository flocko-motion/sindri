package workflow

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSubmitAsksBeforeItTakes: the questions come between the author and the gate, one per call,
// because an agent has no dialogue channel — each `sindri` is its own process, so the answer arrives
// as the next submit and the hub tells it from a summary by how many it already holds.
func TestSubmitAsksBeforeItTakes(t *testing.T) {
	e, ps, _, c := submitEngine(t)

	var first bytes.Buffer
	if _, err := e.CmdSubmit(c, []string{"my work"}, &first); err != nil {
		t.Fatalf("CmdSubmit: %v", err)
	}
	if !strings.Contains(first.String(), "question 1 of 2") {
		t.Fatalf("the first submit should ask, got %q", first.String())
	}
	if _, exists, _ := ps.GetPR("pr-sd-1"); exists {
		t.Error("a PR was opened before the questions were answered")
	}

	// A token answer is refused by length alone — the floor is all the hub can honestly judge.
	var short bytes.Buffer
	if _, err := e.CmdSubmit(c, []string{"yes"}, &short); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(short.String(), "too short") {
		t.Errorf("a one-word answer should not pass, got %q", short.String())
	}

	if code, out := submitAll(t, e, c, "my work"); code != 0 {
		t.Fatalf("an answered submit should land: %d %s", code, out)
	}
	runQueuedGate(t, e)
	if _, exists, _ := ps.GetPR("pr-sd-1"); !exists {
		t.Error("the questions answered, the submit should have gone through")
	}
}

// TestEditingTheCodeDoesNotStartTheQuestionsOver is sudri's loop. The questions send an author into
// the code, and "which test fails if you revert that?" is often unanswerable without writing one —
// so keying the answers on the tree meant answering honestly reset the exercise, while answering
// from memory sailed through. sudri wrote the test, was asked both questions again, and the second
// time replied "unchanged from before". The gate rewarded the shallower answer.
func TestEditingTheCodeDoesNotStartTheQuestionsOver(t *testing.T) {
	e, ps, root, c := submitEngine(t)

	var out bytes.Buffer
	if _, err := e.CmdSubmit(c, []string{"my work"}, &out); err != nil {
		t.Fatal(err)
	}
	answer := "I checked every caller of this helper and each is covered by a test that fails without it."
	out.Reset()
	if _, err := e.CmdSubmit(c, []string{answer}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "question 2 of 2") {
		t.Fatalf("expected the second question, got %q", out.String())
	}

	// The author does what question 2 asks: writes the test that proves the change.
	wt := filepath.Join(root, ".worktrees", "bombur")
	if err := os.WriteFile(filepath.Join(wt, "more_test.go"), []byte("the proving test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if _, err := e.CmdSubmit(c, []string{"TestFoo fails without it — I wrote it this round and checked by reverting."}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "question 1 of 2") {
		t.Errorf("going to the code to answer must not re-ask what was already answered:\n%s", out.String())
	}
	runQueuedGate(t, e)
	if _, exists, _ := ps.GetPR("pr-sd-1"); !exists {
		t.Error("both questions answered, so the submit should have gone through")
	}
}

// TestANewAttemptIsAskedAgain is the other half: a questionnaire closes with the submit it belongs
// to, so a resubmission after a rejection is a fresh round rather than one already paid for.
func TestANewAttemptIsAskedAgain(t *testing.T) {
	e, ps, root, c := submitEngine(t)
	if code, out := submitAll(t, e, c, "my work"); code != 0 {
		t.Fatalf("submit: %d %s", code, out)
	}
	runQueuedGate(t, e)
	if _, exists, _ := ps.GetPR("pr-sd-1"); !exists {
		t.Fatal("precondition: the first submit should have landed")
	}

	if err := e.RejectPR("proj", "pr-sd-1", "another pass, please"); err != nil {
		t.Fatal(err)
	}
	commitIn(t, filepath.Join(root, ".worktrees", "bombur"), "rework.txt", "answering the review\n", "rework")
	var out bytes.Buffer
	if _, err := e.CmdSubmit(c, []string{"reworked after the rejection"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "question 1 of 2") {
		t.Errorf("a new round of review must be asked its own questions:\n%s", out.String())
	}
}

// TestTheRotatingQuestionFollowsTheCommit: one question is anchored because a single failure is over
// half of this project's rejections; the other rotates so the exercise cannot settle into ritual.
// Drawn from the sha, so a retry on the same tree asks the same thing and a REWORK asks something
// else — a fresh angle exactly where the work has just been shown to have a blind spot.
func TestTheRotatingQuestionFollowsTheCommit(t *testing.T) {
	first := submitQuestions("abc123", false)
	if first[0] != qGeneralise {
		t.Errorf("the anchored question should lead, got %q", first[0])
	}
	if got := submitQuestions("abc123", false); got[1] != first[1] {
		t.Errorf("the same commit must draw the same question: %q then %q", first[1], got[1])
	}
	// Across the pool, every question is reachable — a slot nothing ever draws is dead weight.
	seen := map[string]bool{}
	for _, sha := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		seen[submitQuestions(sha, false)[1]] = true
	}
	if len(seen) < 2 {
		t.Errorf("the rotation never rotates: %v", seen)
	}
	// An empty branch is asked the only question it can answer.
	if empty := submitQuestions("abc123", true); empty[0] != qEmpty {
		t.Errorf("an empty diff should be asked to justify itself, got %q", empty[0])
	}
}
