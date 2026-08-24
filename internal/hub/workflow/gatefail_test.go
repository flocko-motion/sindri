package workflow

import (
	"strings"
	"testing"
)

// TestGateFailSaysWhoCausedItDoesNotMatter is dain's loop: it submitted the same failing test three
// times and was about to a fourth, having correctly judged the failure pre-existing. The gate cannot
// act on that, and never said so — so the reply now states the rule and names the way out.
func TestGateFailSaysWhoCausedItDoesNotMatter(t *testing.T) {
	msg := ReplyGateFail("--- FAIL: TestSomethingUnrelated (0.01s)\n")
	for _, want := range []string{
		"only if EVERYTHING passes", // resubmitting an unchanged tree cannot pass
		"pre-existing",              // the exact excuse that produced the loop
		"same result",               // says plainly that a resubmission is wasted
		"sindri escalate",           // the legitimate way out, named
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("gate-failure reply must mention %q:\n%s", want, msg)
		}
	}
	if !strings.Contains(msg, "TestSomethingUnrelated") {
		t.Error("the reply must carry the gate's own output — it is what says what to fix")
	}
}

// TestGateFailDoesNotLectureAboutComments: the reply used to spend five lines on comment length
// whatever had failed, so a broken test was answered with advice about trimming prose. The linter
// explains its own findings; the reply carries them rather than restating them.
func TestGateFailDoesNotLectureAboutComments(t *testing.T) {
	msg := ReplyGateFail("--- FAIL: TestSomethingUnrelated (0.01s)\n")
	for _, unwanted := range []string{"CUT WORDS", "one-liners", "ignore list"} {
		if strings.Contains(msg, unwanted) {
			t.Errorf("a test failure was answered with comment-length advice (%q):\n%s", unwanted, msg)
		}
	}
	// And a real comment-length finding still reaches the agent — through the gate's own words.
	lint := ReplyGateFail("internal/x.go: 2.6 avg over 13 blocks — cut words, don't move them\n")
	if !strings.Contains(lint, "cut words, don't move them") {
		t.Error("the linter's own finding must survive into the reply")
	}
}
