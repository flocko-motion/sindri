package chat

import (
	"strings"
	"testing"
	"time"
)

// TestActingOnTheRoomIsPresence: the lock asks whether a human is at the room, and one who has just
// added a member or said something plainly is. A membership change made from the CLI sends no
// heartbeat of its own, so without this the welcome would land in a room the hub thinks is empty —
// and the agent would never learn it had been added.
func TestActingOnTheRoomIsPresence(t *testing.T) {
	s, _, rec := roomWith(t, "fili")
	if s.Present() {
		t.Fatal("a fresh service has seen no user")
	}
	if err := s.Add("repo", "fili"); err != nil {
		t.Fatal(err)
	}
	if !s.Present() {
		t.Error("adding a member is the user acting on the room")
	}
	if !strings.Contains(rec.linesTo("fili"), "added to the meeting room") {
		t.Errorf("the newcomer must be told it was added: %q", rec.linesTo("fili"))
	}
}

// TestTheRoomLocksWhenTheUserLeaves is the other half: presence is a recent act, not a permanent
// one, so the room goes quiet again shortly after the user stops.
func TestTheRoomLocksWhenTheUserLeaves(t *testing.T) {
	s, _, _ := roomWith(t, "fili")
	s.Heartbeat()
	if !s.Present() {
		t.Fatal("a heartbeat is presence")
	}
	s.mu.Lock()
	s.seen = time.Now().Add(-2 * presenceTTL) // as if the user left a while ago
	s.mu.Unlock()
	if s.Present() {
		t.Error("a room the user left is locked, which is what silences it")
	}
}

// TestNoReminderIntoALockedRoom is the reported bug: dain was told "you're in the meeting room" on
// every relaunch, for a room the user had not touched in ages. Rehydrate runs on every relaunch,
// clear and restart, while membership outlives the meeting — so the cue has to ask whether the room
// can do anything at all before it speaks.
func TestNoReminderIntoALockedRoom(t *testing.T) {
	s, _, _ := roomWith(t, "dain")
	if err := s.Add("repo", "dain"); err != nil {
		t.Fatal(err)
	}
	// Adding was presence, so right now the room is open and the cue is worth having.
	if cue := s.ReminderFor("repo", "dain"); cue != MsgReminder {
		t.Errorf("a member of an open room should be reminded, got %q", cue)
	}

	s.mu.Lock()
	s.seen = time.Now().Add(-2 * presenceTTL) // the user left; the room locked behind them
	s.mu.Unlock()
	if cue := s.ReminderFor("repo", "dain"); cue != "" {
		t.Errorf("a locked room must say nothing on relaunch, got %q", cue)
	}
}

// TestNoReminderForANonMember: the other half of the condition, so an open room does not greet
// every agent the hub happens to relaunch.
func TestNoReminderForANonMember(t *testing.T) {
	s, _, _ := roomWith(t, "dain", "nori")
	if err := s.Add("repo", "dain"); err != nil {
		t.Fatal(err)
	}
	if cue := s.ReminderFor("repo", "nori"); cue != "" {
		t.Errorf("nori is not in the room and should hear nothing, got %q", cue)
	}
}
