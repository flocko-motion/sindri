package workflow

import "testing"

// TestMailReachesAnAgentHoldingWork is dain's stall, twice over: it sat at an empty prompt with an
// unread gate verdict, because holding a task disqualified it from being told. Mail is most urgent
// exactly then — the verdict, the rejection, the cancellation are ABOUT the work in hand.
func TestMailReachesAnAgentHoldingWork(t *testing.T) {
	st, _ := poolFixture(t)
	e := newEngine(st, &stubDeps{root: t.TempDir(), alive: true})
	if !e.reachable("repo", "dvalin") {
		t.Fatal("an agent up and at an empty prompt must be reachable, whatever it holds")
	}
}

// TestAnAgentMidTurnIsStillToldAboutMail inverts what this asserted. Claude Code QUEUES what is typed
// during a turn — its own pane says "Press up to edit queued messages" — so the notice is not lost,
// it arrives as the turn ends, which is the moment the agent can act on it. Waiting for an idle
// prompt meant an agent working for an hour heard nothing, and the news was stale when it did.
//
// Only being THERE still matters: there is nothing to type into a pod that is down.
func TestAnAgentMidTurnIsStillToldAboutMail(t *testing.T) {
	st, _ := poolFixture(t)
	busy := &stubDeps{root: t.TempDir(), alive: true, busy: map[string]bool{"dvalin": true}}
	if e := newEngine(st, busy); !e.reachable("repo", "dvalin") {
		t.Error("a working agent was left untold — it decides when to read, and cannot while it does not know")
	}
	if e := newEngine(st, &stubDeps{root: t.TempDir(), alive: false}); e.reachable("repo", "dvalin") {
		t.Error("an agent that is down must not be nudged — there is nothing to inject into")
	}
}
