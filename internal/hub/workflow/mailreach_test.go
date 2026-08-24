package workflow

import "testing"

// TestMailReachesAnAgentHoldingWork is dain's stall, twice over: it sat at an empty prompt with an
// unread gate verdict, because holding a task disqualified it from being told. Mail is most urgent
// exactly then — the verdict, the rejection, the cancellation are ABOUT the work in hand.
func TestMailReachesAnAgentHoldingWork(t *testing.T) {
	st, _ := poolFixture(t)
	e := New(st, &stubDeps{root: t.TempDir(), alive: true})
	if !e.reachable("repo", "dvalin") {
		t.Fatal("an agent up and at an empty prompt must be reachable, whatever it holds")
	}
}

// TestAnAgentMidTurnIsNotReachable: the one thing that still stops a nudge is delivery — a push into
// a running turn is lost, so it is not sent.
func TestAnAgentMidTurnIsNotReachable(t *testing.T) {
	st, _ := poolFixture(t)
	busy := &stubDeps{root: t.TempDir(), alive: true, busy: map[string]bool{"dvalin": true}}
	if e := New(st, busy); e.reachable("repo", "dvalin") {
		t.Error("an agent mid-turn must not be nudged — the push would be lost")
	}
	if e := New(st, &stubDeps{root: t.TempDir(), alive: false}); e.reachable("repo", "dvalin") {
		t.Error("an agent that is down must not be nudged — there is nothing to inject into")
	}
}
