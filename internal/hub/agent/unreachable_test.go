package agent

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// TestAMessageThePaneNeverShowsIsReported is the whole point: tmux exiting 0 says it accepted the
// keystrokes, and dvalin's rejection was recorded pushed on exactly that while being in neither its
// pane nor its scrollback. Absence is REPORTED — the Enter still goes, and nothing is re-sent, since
// only the caller knows whether its own message may be delivered twice.
func TestAMessageThePaneNeverShowsIsReported(t *testing.T) {
	s, f := tellFixture(t, "dvalin", idlePane)
	f.swallow = true

	err := s.Inject(t.Context(), "proj", "dvalin", "your PR was rejected")
	if err == nil {
		t.Fatal("a message the pane never showed must be reported, not counted as delivered")
	}
	if !strings.Contains(err.Error(), "nothing confirms") {
		t.Errorf("the failure must say what is unconfirmed, got %q", err)
	}
	if len(f.sent) != 1 || f.submits != 1 {
		t.Errorf("the send is completed either way, got sent=%q submits=%d", f.sent, f.submits)
	}
}

// TestPushesNothingShowsBackReadUnreachable: an agent nothing can be said to is holding work nobody
// can redirect, and today it looks perfectly healthy. One late redraw must not cost it its status
// word, so the board only says so after unreachableStrikes in a row — and a message that lands
// withdraws the claim, because it disproves it. The first strike comes from a real injection, so the
// count is wired to the read-back and not merely to itself; the rest are driven, being the same event.
func TestPushesNothingShowsBackReadUnreachable(t *testing.T) {
	s, f := tellFixture(t, "sudri", idlePane)
	f.swallow = true
	_ = s.Inject(t.Context(), "proj", "sudri", "are you there")
	if s.Unreachable("proj", "sudri") {
		t.Fatal("one unconfirmed push is a pane that redrew late, not an agent nothing reaches")
	}
	for i := 1; i < unreachableStrikes; i++ {
		s.strikeUnconfirmed("proj", "sudri")
	}
	if !s.Unreachable("proj", "sudri") {
		t.Fatalf("%d unconfirmed pushes in a row must reach a status of their own", unreachableStrikes)
	}

	f.swallow = false
	if err := s.Inject(t.Context(), "proj", "sudri", "still there?"); err != nil {
		t.Fatalf("a pane that shows the text again must accept it: %v", err)
	}
	if s.Unreachable("proj", "sudri") {
		t.Error("a message that landed disproves the claim, so the strikes must be forgotten")
	}
}

// TestALaunchedPodInheritsNoVerdict: the word is about a session, and a restart is a new one — the
// remedy the board offers for an unreachable agent, which left standing would still read unreachable
// after it. Driven through Launch itself rather than the helper: the count is cleared before the
// pre-flight this fake fails at, so a real call proves the wire and not just the function.
func TestALaunchedPodInheritsNoVerdict(t *testing.T) {
	s, _ := tellFixture(t, "brokkr", idlePane)
	for i := 0; i < unreachableStrikes; i++ {
		s.strikeUnconfirmed("proj", "brokkr")
	}
	if !s.Unreachable("proj", "brokkr") {
		t.Fatal("the fixture must start from the state a launch is meant to clear")
	}
	_ = s.Launch(t.Context(), "proj", "brokkr", false, false, 0, 0, io.Discard)
	if s.Unreachable("proj", "brokkr") {
		t.Error("a fresh pod must not wear the old session's verdict")
	}
}

// TestTheNeedleSurvivesAWrappingPane: the pane wraps, so what is looked for is a SHORT slice — and
// the provenance tag goes first, which spends none of a narrow row on words every message shares.
func TestTheNeedleSurvivesAWrappingPane(t *testing.T) {
	long := "[hub] " + strings.Repeat("a task description that runs on ", 20)
	needle := paneNeedle(long, needleCap+paneChrome)
	if strings.HasPrefix(needle, "[hub]") {
		t.Errorf("the tag must be trimmed before matching, got %q", needle)
	}
	if n := len([]rune(needle)); n != needleCap {
		t.Errorf("needle is %d runes, want it cut to %d so it cannot straddle two lines", n, needleCap)
	}
	// Cut by COLUMNS, not bytes and not runes: half a character matches no pane, and a row is measured
	// in columns — so a message of wide characters yields fewer of them, never a needle that overruns.
	wideMsg := paneNeedle(strings.Repeat("世", 100), needleCap)
	if len([]rune(wideMsg)) == 0 || paneColumns(wideMsg) > needleCap {
		t.Errorf("a wide-character needle is %d columns, want it inside %d", paneColumns(wideMsg), needleCap)
	}
	for _, r := range wideMsg {
		if r != '世' {
			t.Fatalf("needle %q has a halved character in it", wideMsg)
		}
	}
	// A bracket opening a message that is not a tag: the "] " is past tagBytes, so nothing is trimmed.
	notATag := "[" + strings.Repeat("x", tagBytes) + "] first"
	if got := paneNeedle(notATag, needleCap); !strings.HasPrefix(got, "[xxx") {
		t.Errorf("only a SHORT leading tag is trimmed, got %q", got)
	}
	// And one that is: a sender name inside the bound goes, message text and all its brackets stay.
	if got := paneNeedle("[dvalin] [1] of 3 files", needleCap+paneChrome); got != "[1] of 3 files" {
		t.Errorf("a real tag goes and the rest is kept whole, got %q", got)
	}
}

// TestTheNeedleFitsEveryPaneSindriCreates is the check in full rather than at one width. Sessions are
// created at the TUI's preview width (-> tui.previewSize: m.w minus the detail column), so an
// 80-column terminal gives a 29-column pane and a 30-column one gives 9 — every one of which a fixed
// 40-rune needle overruns, reporting a healthy agent unconfirmed on every push the hub makes.
func TestTheNeedleFitsEveryPaneSindriCreates(t *testing.T) {
	looked := 0
	for _, msg := range []string{
		"[hub] the base moved under you, rebase before you submit or the gate refuses it",
		"[user] 世界中のエージェントに送るメッセージ、幅の広い文字ばかりのもの", // wide characters: columns, not runes
		"no tag on this one, just a plain line of message text for the needle",
	} {
		for _, cols := range []int{9, 20, 29, 37, 57, 76, 80, 120} {
			checkNeedleFits(t, msg, cols, &looked)
		}
	}
	// And the skip must not have quietly swallowed every width, which would confirm everything.
	if looked < 15 {
		t.Errorf("only %d cases were actually looked at — confirmation is off almost everywhere", looked)
	}
}

// checkNeedleFits is the invariant itself: whatever this pane can show, the needle either fits one of
// its rows or is not looked for at all. Anything else reports a delivered message as unconfirmed.
func checkNeedleFits(t *testing.T, msg string, cols int, looked *int) {
	t.Helper()
	pane := drawn(cols, msg)
	needle := paneNeedle(msg, paneWidth(pane)-paneChrome)
	if needle == "" {
		return // too narrow to prove anything: skipped, which is the safe direction
	}
	*looked++
	if !strings.Contains(pane, needle) {
		t.Errorf("pane %d cols: needle %q is wider than a row, so every push reads unconfirmed", cols, needle)
	}
}

// TestNothingToLookForIsSkippedNotFailed pins every degenerate input to the SAME answer: no needle,
// so confirmation is skipped. Each of these is a pane or a message that can prove nothing, and the
// one outcome that must never come out of "cannot tell" is the strike that ends in "unreachable".
func TestNothingToLookForIsSkippedNotFailed(t *testing.T) {
	// A capture that came back empty, or holds nothing but blank rows: no width to derive from.
	for _, pane := range []string{"", "\n\n"} {
		if got := paneNeedle("a message with plenty of words in it", paneWidth(pane)-paneChrome); got != "" {
			t.Errorf("pane %q gives budget %d and needle %q, want none", pane, paneWidth(pane)-paneChrome, got)
		}
	}
	// A budget of nothing, and one gone negative on a pane narrower than its own chrome.
	for _, budget := range []int{0, -8, needleFloor - 1} {
		if got := paneNeedle("a message with plenty of words in it", budget); got != "" {
			t.Errorf("budget %d gives needle %q, want none", budget, got)
		}
	}
	// A message with nothing in it, and one that is only a provenance tag.
	for _, msg := range []string{"", "   ", "[hub] ", "[hub] \nall of it on the second line"} {
		if got := paneNeedle(msg, needleCap+paneChrome); got != "" {
			t.Errorf("message %q gives needle %q, want none", msg, got)
		}
	}
	if paneColumns("") != 0 {
		t.Error("an empty string is no columns wide")
	}
	// And the outcome nobody set: the zero readback must be the one that claims nothing, or a path
	// added later that forgets to set it reports every message as delivered.
	if readback(0) != paneCannotSay {
		t.Error("the zero readback must claim nothing, not arrival")
	}
}

// TestAZeroServiceAnswersRatherThanPanics: the strike map is built on first use, so every read of it
// comes before that on a Service nobody has struck — including the board's, once per agent per render.
func TestAZeroServiceAnswersRatherThanPanics(t *testing.T) {
	var s Service
	if s.Unreachable("proj", "nobody") {
		t.Error("an agent with no strikes recorded is not unreachable")
	}
	s.clearStrikes("proj", "nobody") // delete from a nil map: a landed push on a fresh hub
	if s.Unreachable("proj", "nobody") {
		t.Error("clearing what was never counted must leave it uncounted")
	}
}

// TestAPaneTooNarrowStillConfirms drives the real injection at the width an 80-column terminal makes.
// The unit check above proves the needle fits; this proves the derivation is actually wired into the
// send, which is where a needle picked instead of measured did its damage.
func TestAPaneTooNarrowStillConfirms(t *testing.T) {
	for _, cols := range []int{9, 29, 37} {
		s, f := tellFixture(t, "thrain", idlePaneAt(cols))
		f.cols = cols
		err := s.Inject(t.Context(), "proj", "thrain", "[hub] the base moved under you, rebase before you submit")
		if err != nil {
			t.Errorf("pane %d cols: %v", cols, err)
		}
		if s.Unreachable("proj", "thrain") {
			t.Errorf("pane %d cols: a narrow terminal must not make an agent unreachable", cols)
		}
	}
}

// TestAPaneThatCannotSayIsNotTakenAsArrival is the third outcome, and the reason there are three.
// A pane too narrow to carry a distinctive slice, and a capture that came back empty, both prove
// NOTHING — recorded as arrival they report a vanished message as landed and, worse, wipe the strikes
// already standing, so the third one never arrives and the evidence for it is gone.
func TestAPaneThatCannotSayIsNotTakenAsArrival(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(*fakeRuntime)
	}{
		{"a pane too narrow to prove anything", func(f *fakeRuntime) { f.cols, f.pane = 9, idlePaneAt(9) }},
		{"a capture that came back empty", func(f *fakeRuntime) { f.blank = true }},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, f := tellFixture(t, "nain", idlePane)
			f.swallow = true // whatever is typed, the pane will not show it
			for i := 0; i < unreachableStrikes-1; i++ {
				s.strikeUnconfirmed("proj", "nain")
			}
			c.setup(f)
			if err := s.Inject(t.Context(), "proj", "nain", "[hub] your PR was rejected"); err != nil {
				t.Logf("reported: %v", err) // an unreadable pane is reported; a narrow one is not
			}
			if s.Unreachable("proj", "nain") {
				t.Error("a pane that can say nothing must not be the strike that condemns an agent")
			}
			s.strikeUnconfirmed("proj", "nain")
			if !s.Unreachable("proj", "nain") {
				t.Error("the standing strikes were WIPED by a send that proved nothing — the evidence a " +
					"third would have rested on is gone")
			}
		})
	}
}

// TestABlankCaptureInPassingDoesNotEndTheSearch: a capture that came back empty is NO reading, not a
// reading of a narrow pane, so the search waits for a real one. Settling on the first blank would
// short-circuit an ordinary wide pane into proving nothing, on nothing worse than one dropped read.
func TestABlankCaptureInPassingDoesNotEndTheSearch(t *testing.T) {
	s, f := tellFixture(t, "dori", idlePane)
	s.strikeUnconfirmed("proj", "dori") // something standing, so a real confirmation is visible
	// Two: the signed-out guard takes the first capture, so this leaves one blank for the read-back.
	f.blankFor = 2

	if err := s.Inject(t.Context(), "proj", "dori", "[hub] the base moved under you"); err != nil {
		t.Fatalf("a pane blank for one read must still be looked at again: %v", err)
	}
	if _, struck := s.reach.strikes[lcKey{"proj", "dori"}]; struck {
		t.Error("the message was there on the second look, so this was a confirmation and must clear")
	}
}

// TestANewlineNeverEntersTheNeedle: a needle spanning a line break can never match, because the pane
// draws the second line as its own row behind the input box's chrome. Nothing on the delivery path
// strips newlines, so every multi-line `agent tell` would report unconfirmed — and three of those
// would put a healthy agent at "unreachable", the very outcome the needle is kept short to avoid.
func TestANewlineNeverEntersTheNeedle(t *testing.T) {
	for _, text := range []string{
		"[user] Heads up:\nthe base moved under you, rebase before you submit.",
		"[hub] Two things:\n- the gate is red\n- I rebased",
	} {
		needle := paneNeedle(text, needleCap+paneChrome)
		if strings.Contains(needle, "\n") {
			t.Errorf("needle %q spans a line break, which no pane renders as one row", needle)
		}
		if !strings.Contains(drawn(paneWide, text), needle) {
			t.Errorf("a drawn pane must contain the needle for %q, got needle %q", text, needle)
		}
	}
	// A first line with nothing distinctive left is skipped, as an empty message is — never failed.
	if got := paneNeedle("[hub] \n the answer is on the second line", needleCap+paneChrome); got != "" {
		t.Errorf("nothing distinctive on the first line must yield no needle, got %q", got)
	}
}

// TestAMultiLineMessageConfirmsThroughARealPane runs the same edge end to end — real injection, real
// needle, a pane that draws rather than echoes. The rune trim hides it whenever the first line is
// long: the hub's own gate verdict opens with 45 runes before its newline and clears needleRunes by
// five, so rewording that one sentence shorter is all it would take to break every gate push.
func TestAMultiLineMessageConfirmsThroughARealPane(t *testing.T) {
	s, _ := tellFixture(t, "nori", idlePane)
	msg := "[hub] Two things:\n- the gate is red\n- I rebased you onto the current base"
	if err := s.Inject(t.Context(), "proj", "nori", msg); err != nil {
		t.Fatalf("a message the pane drew over three rows must confirm: %v", err)
	}
	if s.Unreachable("proj", "nori") {
		t.Error("a delivered message must never count toward the word")
	}
}

// TestAClearIsConfirmedBeforeItWipesThePane pins the read-back's POSITION, the crux of this change:
// between the literal and the Enter. /clear empties the transcript its own echo landed in, so a
// capture taken after the Enter finds nothing and reports a clear that worked perfectly as failed.
func TestAClearIsConfirmedBeforeItWipesThePane(t *testing.T) {
	s, f := tellFixture(t, "fili", idlePane+"some earlier turn\n")
	if err := s.Inject(t.Context(), "proj", "fili", "/clear"); err != nil {
		t.Fatalf("/clear must be confirmed while it is still on screen: %v", err)
	}
	if strings.Contains(f.pane, "/clear") {
		t.Fatal("the fake must wipe its pane on the Enter, or this proves nothing about the ordering")
	}
	if s.Unreachable("proj", "fili") || f.submits != 1 {
		t.Errorf("a confirmed clear strikes nothing, got submits=%d", f.submits)
	}
}

// TestABlockedDialogIsNeverStruck: a selection form consumes keystrokes without drawing them, and
// `agent tell` is the documented remedy for "blocked". Confirming there would fail every answer and,
// on the third, replace the word naming that remedy with one naming none.
func TestABlockedDialogIsNeverStruck(t *testing.T) {
	s, f := tellFixture(t, "oin", "Do you want to proceed?\n❯ Yes\n  No\nesc to cancel")
	f.swallow = true // the dialog takes the keys and draws nothing, which is the point
	if err := s.Inject(t.Context(), "proj", "oin", "yes, and keep going after that"); err != nil {
		t.Fatalf("answering a dialog must not report failure: %v", err)
	}
	if s.Unreachable("proj", "oin") {
		t.Error("a dialog that draws nothing is not an agent nothing reaches")
	}
}

// TestADialogOpenedAfterTheLastReadingIsStillSeen pins WHEN the reading is taken: after the
// keystrokes, never before them. Observe memoises for runtimeTTL, so a pane sampled while the agent
// was idle answers for a dialog opened since — which is every back-to-back injection. SetModel sends
// /model and then its instruction with nothing in between, and its own comment says /model opens a
// dialog that swallows what follows; decided on the earlier reading, that second send is struck and
// its error RETURNED, so a switch that worked reports failure. Compact has the same shape, and there
// the error also skips ForgetContext and the "compact" log line — the symptom this task was filed on.
func TestADialogOpenedAfterTheLastReadingIsStillSeen(t *testing.T) {
	s, f := tellFixture(t, "balin", idlePane)
	if err := s.Inject(t.Context(), "proj", "balin", "/model sonnet"); err != nil {
		t.Fatalf("the first send lands and fills the standing reading with idle: %v", err)
	}
	// The dialog opens INSIDE runtimeTTL, so any memoised reading still says idle.
	f.pane = "Do you want to proceed?\n❯ Yes\n  No\nesc to cancel"
	f.swallow = true
	if err := s.Inject(t.Context(), "proj", "balin", "carry on with the task once switched"); err != nil {
		t.Fatalf("a dialog opened since the last reading must still be seen: %v", err)
	}
	if s.Unreachable("proj", "balin") {
		t.Error("a dialog is not an agent nothing reaches, however stale the standing reading")
	}
}

// TestAnUnconfirmedSendIsNotLoggedAsSkipped: the log is what a user reconstructs a silence from, and
// "typed and submitted, never seen" is a different fact from "nothing was typed". Filing both under
// inject-skipped would erase the one distinction this change exists to draw.
func TestAnUnconfirmedSendIsNotLoggedAsSkipped(t *testing.T) {
	s, f := tellFixture(t, "bifur", idlePane)
	f.swallow = true
	if err := s.InjectWhenReady(t.Context(), "proj", "bifur", "the base moved under you"); err == nil {
		t.Fatal("an unconfirmed send must still be reported")
	}
	evs, err := s.store.For("proj").Events("bifur", 10)
	if err != nil || len(evs) == 0 {
		t.Fatalf("want the attempt on the record, got %d event(s), err %v", len(evs), err)
	}
	if evs[0].Type != "inject-unconfirmed" {
		t.Errorf("logged as %q, want inject-unconfirmed — the text WAS typed", evs[0].Type)
	}
}

// TestACancelledCallerStrikesNothing: the read-back can end because nobody is waiting for it any
// more, and that is a fact about the caller. Counting it would let a client disconnecting three
// times mark a healthy agent unreachable.
func TestACancelledCallerStrikesNothing(t *testing.T) {
	s, f := tellFixture(t, "gloin", idlePane)
	f.swallow = true
	for i := 0; i <= unreachableStrikes; i++ {
		// A deadline the literal send beats and the read-back does not, so the abandonment happens
		// where it is decided. The fake honours ctx, so this is the real path and not a fixture's.
		ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
		err := s.Inject(ctx, "proj", "gloin", "are you there")
		cancel()
		if err == nil {
			t.Fatal("an abandoned read-back confirms nothing, so it must still be reported")
		}
		if s.Unreachable("proj", "gloin") {
			t.Fatal("no number of cancelled callers may earn an agent the word")
		}
	}
	// And the Enter still went every time: a line left unsubmitted would go later, in front of
	// whatever is typed next.
	if f.submits != unreachableStrikes+1 {
		t.Errorf("submitted %d of %d sends — an abandoned caller must not strand text in the input box",
			f.submits, unreachableStrikes+1)
	}
}
