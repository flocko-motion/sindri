package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
)

// joinedRoom is roomWith plus the members actually in the room, which is the state a close acts on.
func joinedRoom(t *testing.T, names ...string) (*Service, *store.Store, *recorder) {
	t.Helper()
	s, st, rec := roomWith(t, names...)
	for _, n := range names {
		if err := s.Add("repo", n); err != nil {
			t.Fatalf("add %s: %v", n, err)
		}
	}
	return s, st, rec
}

// TestCloseEmptiesTheRoomAndKeepsTheTranscript is the whole point: membership is durable and only an
// explicit removal ever dropped it, so two agents sat in a room for days. Closing ends it — and
// leaves the history, because reading a finished meeting is a different question from clearing it.
func TestCloseEmptiesTheRoomAndKeepsTheTranscript(t *testing.T) {
	s, st, rec := joinedRoom(t, "fili", "kili")
	if _, err := s.Say("shall we"); err != nil {
		t.Fatal(err)
	}

	n, err := s.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if n != 2 {
		t.Errorf("closed a room of two and removed %d", n)
	}
	members, err := st.ChatMembers()
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Errorf("the room should be empty, still holds %d", len(members))
	}
	for _, name := range []string{"fili", "kili"} {
		if !strings.Contains(rec.linesTo(name), "closed") {
			t.Errorf("%s was removed without being told: %q", name, rec.linesTo(name))
		}
	}
	msgs, err := st.ChatTranscript(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) == 0 {
		t.Fatal("closing must not clear the history — `meeting new` owns that")
	}
	if last := msgs[len(msgs)-1].Body; !strings.Contains(last, "closed") {
		t.Errorf("the transcript should record why the room emptied, ends with %q", last)
	}
}

// TestClosingAnEmptyRoomDoesNothing: it is safe to press twice, and a room nobody joined has no
// meeting to end — so no notice and no transcript line either.
func TestClosingAnEmptyRoomDoesNothing(t *testing.T) {
	s, st, _ := joinedRoom(t)
	n, err := s.Close()
	if err != nil || n != 0 {
		t.Fatalf("closing an empty room: n=%d err=%v", n, err)
	}
	msgs, _ := st.ChatTranscript(0)
	if len(msgs) != 0 {
		t.Errorf("nothing happened, so nothing should be recorded: %+v", msgs)
	}
}

// TestAutoCloseWaitsForTheIdleHour: the room closes itself only once the meeting is plainly over.
// Measured from the last MESSAGE — a room still being talked in stays open however the presence
// lock happens to read.
func TestAutoCloseWaitsForTheIdleHour(t *testing.T) {
	s, _, _ := joinedRoom(t, "fili")
	if _, err := s.Say("still going"); err != nil {
		t.Fatal(err)
	}
	if n, err := s.CloseIfIdle(); err != nil || n != 0 {
		t.Fatalf("a room spoken in just now must stay open: n=%d err=%v", n, err)
	}

	// An hour later, with nothing further said, it is over. Measured against a given moment rather
	// than by waiting, which is why closeIfIdleAt takes one.
	n, err := s.closeIfIdleAt(time.Now().Add(2 * time.Hour))
	if err != nil {
		t.Fatalf("closeIfIdleAt: %v", err)
	}
	if n != 1 {
		t.Errorf("an hour of silence should have closed the room, removed %d", n)
	}
}

// TestAutoCloseDoesNotWakeAnybody is the one real cost of closing a room with members in it: six
// members means six injections. A deliberate close earns that; an automatic one must not interrupt
// an agent to tell it a meeting it had forgotten about is over.
func TestAutoCloseDoesNotWakeAnybody(t *testing.T) {
	s, _, rec := joinedRoom(t, "fili")
	if _, err := s.Say("that's all"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.closeIfIdleAt(time.Now().Add(2 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(rec.whenReady["fili"], "\n"), "closed") {
		t.Errorf("the notice must wait for an idle prompt, sent: %q", rec.linesTo("fili"))
	}
}

// TestANeverUsedRoomIsNotClosed: with nothing ever said there is no meeting, so there is no silence
// to measure and nothing to announce.
func TestANeverUsedRoomIsNotClosed(t *testing.T) {
	s, _, _ := joinedRoom(t)
	if n, err := s.CloseIfIdle(); err != nil || n != 0 {
		t.Fatalf("an unused room: n=%d err=%v", n, err)
	}
}
