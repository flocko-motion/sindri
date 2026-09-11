package pr

import (
	"bytes"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSubmitAsksBeforeItTakes: the questions come between the author and the gate, and they are
// written down the moment the submit is asked for — so what the author owes is a record rather than
// a position in a conversation, and a hub that dies mid-interview resumes on the same questions.
func TestSubmitAsksBeforeItTakes(t *testing.T) {
	a, ps, _, c := submitEngine(t)

	var first bytes.Buffer
	if _, err := a.CmdSubmit(c, []string{"my work"}, &first); err != nil {
		t.Fatalf("CmdSubmit: %v", err)
	}
	tree, rows, err := ps.OpenSubmitAnswers("bombur")
	if err != nil {
		t.Fatal(err)
	}
	if tree == "" || !InterviewOpen(rows) {
		t.Fatalf("the submit should have opened an interview, got %q with %d rows", tree, len(rows))
	}
	if seq, _, of, _ := Standing(rows); seq != 1 || of != 2 {
		t.Errorf("the first question should be 1 of 2, got %d of %d", seq, of)
	}
	if _, exists, _ := ps.GetPR("pr-sd-1"); exists {
		t.Error("a PR was opened before the questions were answered")
	}

	// A token answer is refused by length alone — the floor is all the hub can honestly judge.
	var short bytes.Buffer
	if _, err := a.CmdSubmit(c, []string{"yes"}, &short); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(short.String(), "too short") {
		t.Errorf("a one-word answer should not pass, got %q", short.String())
	}
	if _, after, _ := ps.OpenSubmitAnswers("bombur"); !InterviewOpen(after) {
		t.Error("a refused answer must leave the question standing")
	}

	if code, out := submitAll(t, a, c, "my work"); code != 0 {
		t.Fatalf("an answered submit should land: %d %s", code, out)
	}
	runQueuedGate(t, a)
	if _, exists, _ := ps.GetPR("pr-sd-1"); !exists {
		t.Error("the questions answered, the submit should have gone through")
	}
}

// TestTheQuestionsAreFixedWhenTheInterviewOpens is sudri's loop, closed at the source. The questions
// once redrew themselves from the CURRENT tree on every call, so an author who went to the code to
// answer honestly was asked everything again while one answering from memory sailed through — the
// gate rewarded the shallower answer. They are written down once now, and answering one moves the
// interview on rather than restarting it.
func TestTheQuestionsAreFixedWhenTheInterviewOpens(t *testing.T) {
	a, ps, root, c := submitEngine(t)

	var out bytes.Buffer
	if _, err := a.CmdSubmit(c, []string{"my work"}, &out); err != nil {
		t.Fatal(err)
	}
	_, opened, err := ps.OpenSubmitAnswers("bombur")
	if err != nil {
		t.Fatal(err)
	}
	answer := "I checked every caller of this helper and each is covered by a test that fails without it."
	if _, err := a.CmdSubmit(c, []string{answer}, &out); err != nil {
		t.Fatal(err)
	}
	_, rows, err := ps.OpenSubmitAnswers("bombur")
	if err != nil {
		t.Fatal(err)
	}
	seq, _, of, standing := Standing(rows)
	if !standing || seq != 2 || of != 2 {
		t.Fatalf("an answer should move the interview on, got %d of %d (standing %v)", seq, of, standing)
	}
	if rows[1].Question != opened[1].Question || rows[2].Question != opened[2].Question {
		t.Error("answering redrew the questions, which is how answering honestly cost an author the round")
	}
	// The tree the author is answering about stays exactly as it was: the answers describe it, so
	// moving it abandons the whole submit (-> worker/interviewing).
	if err := os.WriteFile(filepath.Join(root, ".worktrees", "bombur", "more_test.go"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, after, _ := ps.OpenSubmitAnswers("bombur"); after[1].Answer != rows[1].Answer {
		t.Error("an edit must not reach into the record of what has already been answered")
	}
}

// TestANewAttemptIsAskedAgain is the other half: an interview closes with the submit it belongs to,
// so a resubmission after a rejection is a fresh round rather than one already paid for.
func TestANewAttemptIsAskedAgain(t *testing.T) {
	a, ps, root, c := submitEngine(t)
	if code, out := submitAll(t, a, c, "my work"); code != 0 {
		t.Fatalf("submit: %d %s", code, out)
	}
	runQueuedGate(t, a)
	if _, exists, _ := ps.GetPR("pr-sd-1"); !exists {
		t.Fatal("precondition: the first submit should have landed")
	}

	if err := a.RejectPR(proj, "pr-sd-1", "another pass, please"); err != nil {
		t.Fatal(err)
	}
	flowtest.CommitIn(t, filepath.Join(root, ".worktrees", "bombur"), "rework.txt", "answering the review\n", "rework")
	var out bytes.Buffer
	if _, err := a.CmdSubmit(c, []string{"reworked after the rejection"}, &out); err != nil {
		t.Fatal(err)
	}
	_, rows, err := ps.OpenSubmitAnswers("bombur")
	if err != nil {
		t.Fatal(err)
	}
	if seq, _, of, standing := Standing(rows); !standing || seq != 1 || of != 2 {
		t.Errorf("a new round of review must be asked its own questions, got %d of %d (standing %v)", seq, of, standing)
	}
}

// TestTheRotatingQuestionFollowsTheTree: one question is anchored because a single failure is over
// half of this project's rejections; the other rotates so the exercise cannot settle into ritual.
// Drawn from the tree, so a retry on the same one asks the same thing and a REWORK asks something
// else — a fresh angle exactly where the work has just been shown to have a blind spot.
func TestTheRotatingQuestionFollowsTheTree(t *testing.T) {
	first := SubmitQuestions("abc123", false)
	if first[0] != QGeneralise {
		t.Errorf("the anchored question should lead, got %q", first[0])
	}
	if got := SubmitQuestions("abc123", false); got[1] != first[1] {
		t.Errorf("the same tree must draw the same question: %q then %q", first[1], got[1])
	}
	// Across the pool, every question is reachable — a slot nothing ever draws is dead weight.
	seen := map[string]bool{}
	for _, tree := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		seen[SubmitQuestions(tree, false)[1]] = true
	}
	if len(seen) < 2 {
		t.Errorf("the rotation never rotates: %v", seen)
	}
	// An empty branch is asked the only question it can answer.
	if empty := SubmitQuestions("abc123", true); empty[0] != QEmpty {
		t.Errorf("an empty diff should be asked to justify itself, got %q", empty[0])
	}
}
