package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mailStore opens a throwaway store with two projects' mail in it.
func mailStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestReadingMailMARKSIt is the constraint the whole Mail view rests on: reading a message sets a
// stamp and leaves the row, so the mailbox is the record of what an agent was told. A queue that
// drained on read would leave nothing to show, and no way to answer "was it ever told?".
func TestReadingMailMARKSIt(t *testing.T) {
	s := mailStore(t)
	ps := s.For("proj")
	m, err := ps.AddMail("dvalin", "reviewer", "rejected: the gate is missing", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID == 0 || m.SentAt == "" || !m.Pushed {
		t.Fatalf("a stored message should come back with its id, its time and its push flag: %+v", m)
	}
	if m.Read() {
		t.Error("a new message is unread")
	}
	if err := ps.MarkMailRead(m.ID); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.MailByID(m.ID)
	if err != nil || !ok {
		t.Fatalf("the message must still exist after being read (ok=%v, err=%v)", ok, err)
	}
	if !got.Read() {
		t.Error("reading should stamp read_at")
	}
	if got.Body != m.Body {
		t.Errorf("the body must survive being read: %q", got.Body)
	}
	// The first stamp is the one that answers "when did it learn", so a second read keeps it.
	first := got.ReadAt
	if err := ps.MarkMailRead(m.ID); err != nil {
		t.Fatal(err)
	}
	if again, _, _ := s.MailByID(m.ID); again.ReadAt != first {
		t.Errorf("re-reading moved the read stamp from %q to %q", first, again.ReadAt)
	}
}

// TestALandedPushGoesQuietThenSpeaksAgain: a push that landed counts as told, so the agent is not
// nudged about something it just received. But "send-keys was accepted" is not "the agent read it" —
// dvalin's rejection carried pushed=1 and never reached its pane — so once the message has sat
// unread past the interval it counts again, and the nudge repeats until it is read.
func TestALandedPushGoesQuietThenSpeaksAgain(t *testing.T) {
	s := mailStore(t)
	ps := s.For("proj")
	m, err := ps.AddMail("dvalin", "hub", "already pushed", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddMail("dvalin", "hub", "never pushed", false, 0); err != nil {
		t.Fatal(err)
	}
	if err := ps.LogMail(m.ID, MailPushLanded, ""); err != nil {
		t.Fatal(err)
	}

	unannounced, unread, err := ps.UnannouncedMail("dvalin", time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if unread != 2 {
		t.Errorf("unread = %d, want 2 — both messages are still unread", unread)
	}
	if unannounced != 1 {
		t.Errorf("unannounced = %d, want 1 — the freshly pushed one needs no second word", unannounced)
	}

	// The same mailbox, asked about a moment further on: the push is stale, so it is due again.
	if unannounced, _, err = ps.UnannouncedMail("dvalin", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if unannounced != 2 {
		t.Errorf("unannounced = %d, want 2 — an unread message is told again once the push goes stale", unannounced)
	}
}

// TestMailIsFleetWideAndNewestFirst: the view spans agents and repos, which is what makes it a Mail
// section rather than a second per-agent timeline.
func TestMailIsFleetWideAndNewestFirst(t *testing.T) {
	s := mailStore(t)
	if _, err := s.For("one").AddMail("dvalin", "hub", "first", false, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.For("two").AddMail("nori", "user", "second", false, 0); err != nil {
		t.Fatal(err)
	}
	all, err := s.AllMail(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Body != "second" {
		t.Fatalf("AllMail should return both projects' mail, newest first: %+v", all)
	}
	if all[0].Project != "two" || all[1].Agent != "dvalin" {
		t.Errorf("each row carries its own project and recipient: %+v", all)
	}
}

// TestMailWindowKeepsTheRecentReadEnd: the window bounds the RENDER, not the record, so once
// everything in it is read it just has to keep the newest. Ties on read_at (as here, all read within
// the same instant) fall back to id DESC, so this holds regardless of when each was read — the room
// LIMIT is what bounds the read half, "just read" included (-> AllMail's own room==0 case). The
// tallies must still count everything, or a view would present its window as the history and the
// badge would stop rising once the mailbox outgrew it.
func TestMailWindowKeepsTheRecentReadEnd(t *testing.T) {
	s := mailStore(t)
	ps := s.For("proj")
	var ids []int64
	for i := 0; i < 5; i++ {
		m, err := ps.AddMail("dvalin", "hub", strings.Repeat("x", i+1), false, 0)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.ID)
	}
	for _, id := range ids {
		if err := ps.MarkMailRead(id); err != nil {
			t.Fatal(err)
		}
	}
	window, err := s.AllMail(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(window) != 2 || window[0].Body != "xxxxx" {
		t.Fatalf("a window of 2 should be the newest two: %+v", window)
	}
	total, unread, userUnread, byProject, err := s.MailTallies()
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || unread != 0 || byProject["proj"] != 0 {
		t.Errorf("tallies count the whole mailbox, got total=%d unread=%d byProject=%v", total, unread, byProject)
	}
	// None of it is addressed to the user, and that share comes out of the same single pass.
	if userUnread != 0 {
		t.Errorf("the user's own unread = %d, want 0 — this is all agent traffic", userUnread)
	}
}

// TestUnreadRidesThroughTheWindowRegardlessOfAge is sd-ca8929's own bug: the window used to bound
// EVERYTHING by recency, so an unread message old enough to fall out of it was counted by the badge
// and unreachable by every list. Read mail is what a window is safe to drop — it already has an
// owner who dealt with it — so only that end may shrink.
func TestUnreadRidesThroughTheWindowRegardlessOfAge(t *testing.T) {
	s := mailStore(t)
	ps := s.For("proj")
	oldestUnread, err := ps.AddMail("dvalin", "hub", "never read", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Everything after it is read, newer, and plentiful enough to fill (and overflow) a window of 2.
	for i := 0; i < 4; i++ {
		m, err := ps.AddMail("dvalin", "hub", strings.Repeat("y", i+1), false, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := ps.MarkMailRead(m.ID); err != nil {
			t.Fatal(err)
		}
	}
	window, err := s.AllMail(2)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range window {
		if m.ID == oldestUnread.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("the oldest message is the only unread one — it must ride through a window of 2 anyway: %+v", window)
	}
	// One unread + the room left (1) filled by the single newest read message, newest first: the
	// newest read message outranks the much older unread one, which trails at the very end.
	if len(window) != 2 || window[0].Body != "yyyy" || window[1].ID != oldestUnread.ID {
		t.Fatalf("the newest read message, then the old unread one trailing behind it: %+v", window)
	}
}

// TestUnreadCanOutgrowTheWindowEntirely: when unread alone exceeds the limit, every one of them still
// comes back — there is no read mail left to make room for, and the window growing past its usual
// size is exactly the "fleet problem the badge is correctly reporting" the decision names.
func TestUnreadCanOutgrowTheWindowEntirely(t *testing.T) {
	s := mailStore(t)
	ps := s.For("proj")
	for i := 0; i < 3; i++ {
		if _, err := ps.AddMail("dvalin", "hub", strings.Repeat("z", i+1), false, 0); err != nil {
			t.Fatal(err)
		}
	}
	window, err := s.AllMail(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(window) != 3 {
		t.Fatalf("all 3 are unread and none may hide, want a window of 3, got %d: %+v", len(window), window)
	}
}

// TestASaturatedWindowCanStillLoseAJustReadMessage pins AllMail's own deliberate exception (round 3):
// once unread alone has already spent the whole limit, room is 0 and no read message rides, "just
// read" included. This is the one case round 2's promise does not reach — accepted on purpose, since
// removing it would mean either an unbounded read half (round 3's actual blocker) or a dynamic bound
// with no honest ceiling; a fleet with this much simultaneously unread has bigger problems than one
// row's timing.
func TestASaturatedWindowCanStillLoseAJustReadMessage(t *testing.T) {
	s := mailStore(t)
	ps := s.For("proj")
	var ids []int64
	for i := 0; i < 3; i++ {
		m, err := ps.AddMail("dvalin", "hub", strings.Repeat("s", i+1), false, 0)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.ID)
	}
	if err := ps.MarkMailRead(ids[0]); err != nil { // room is now 0: the other two unread fill limit=2
		t.Fatal(err)
	}
	window, err := s.AllMail(2)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range window {
		if m.ID == ids[0] {
			t.Fatalf("the saturated case is documented as NOT riding the just-read message — if this now "+
				"passes, AllMail's own room==0 comment is stale: %+v", window)
		}
	}
	if len(window) != 2 {
		t.Fatalf("the remaining unread pair should fill the window exactly, got %+v", window)
	}
}

// TestARecentlyReadOldMessageStillRidesTheWindow is review round 2's blocker: reading an old unread
// message used to drop it from the very board the read happened on, since its id fell outside the
// newest-by-id read set the next AllMail computed. MailActive's own promise — a message read a moment
// ago stays on screen instead of vanishing as it is read (-> api.MatchesMailFilter) — has to hold for
// an old message too, not just an id-recent one.
func TestARecentlyReadOldMessageStillRidesTheWindow(t *testing.T) {
	s := mailStore(t)
	ps := s.For("proj")
	old, err := ps.AddMail("dvalin", "hub", "sent long ago", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Plenty of newer read mail, so old's id falls well outside a window of 2 by id-recency alone.
	for i := 0; i < 5; i++ {
		m, err := ps.AddMail("dvalin", "hub", strings.Repeat("n", i+1), false, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := ps.MarkMailRead(m.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := ps.MarkMailRead(old.ID); err != nil { // read last — "a moment ago" — its id stays oldest
		t.Fatal(err)
	}
	// MarkMailRead's clock is second-resolution (RFC3339, every timestamp in this store's own format),
	// so a fast test ties with the reads just above and falls back to id DESC — the opposite of what
	// "read last" is meant to prove. Stamp old's read_at a second ahead, the gap a slower reader would
	// produce for real.
	future := time.Now().UTC().Add(time.Second).Format(time.RFC3339)
	if _, err := s.db.Exec(`UPDATE mail SET read_at=? WHERE id=?`, future, old.ID); err != nil {
		t.Fatal(err)
	}
	window, err := s.AllMail(2)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range window {
		if m.ID == old.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("a message just read must not vanish for having an old id: %+v", window)
	}
}

// TestMailMergesUnreadAndReadNewestFirst: AllMail promises newest first, so an old unread message and
// a newer read one must interleave by id — otherwise every unread row would sort ahead of a read row
// that is actually more recent, and the list would stop reading top to bottom as "newest first".
func TestMailMergesUnreadAndReadNewestFirst(t *testing.T) {
	s := mailStore(t)
	ps := s.For("proj")
	unreadOld, err := ps.AddMail("dvalin", "hub", "old, unread", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	readMid, err := ps.AddMail("dvalin", "hub", "middle, read", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.MarkMailRead(readMid.ID); err != nil {
		t.Fatal(err)
	}
	unreadNew, err := ps.AddMail("dvalin", "hub", "newest, unread", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	window, err := s.AllMail(0)
	if err != nil {
		t.Fatal(err)
	}
	var got []int64
	for _, m := range window {
		got = append(got, m.ID)
	}
	want := []int64{unreadNew.ID, readMid.ID, unreadOld.ID}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("AllMail should merge by id, newest first, got %v want %v", got, want)
	}
}

// TestMailSurvivesAReopen: durability is the point of mail against injection — a message must not be
// lost to a hub restart, which is exactly when an agent is not there to be injected into.
func TestMailSurvivesAReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.For("proj").AddMail("dvalin", "hub", "merged pr-sd-1", true, 0); err != nil {
		t.Fatal(err)
	}
	s.Close()

	again, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer again.Close()
	all, err := again.AllMail(0)
	if err != nil || len(all) != 1 || all[0].Body != "merged pr-sd-1" {
		t.Fatalf("mail must survive a restart, got %+v (err %v)", all, err)
	}
}

// TestTheUsersShareComesFromTheSamePass: the board is rebuilt on every notify for every client and the
// mailbox grows for the life of the machine, so the user's own unread must not cost a second scan.
func TestTheUsersShareComesFromTheSamePass(t *testing.T) {
	s := mailStore(t)
	ps := s.For("proj")
	for _, m := range []struct{ to, from string }{
		{"dvalin", "hub"}, {"dvalin", "hub"}, {"user", "dvalin"}, {"user", "nori"},
	} {
		if _, err := ps.AddMail(m.to, m.from, "a message", false, 0); err != nil {
			t.Fatal(err)
		}
	}
	total, unread, userUnread, _, err := s.MailTallies()
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 || unread != 4 || userUnread != 2 {
		t.Errorf("tallies = total %d, unread %d, user %d; want 4, 4 and 2", total, unread, userUnread)
	}
	// Reading one of the user's own moves only that number.
	mail, _ := s.AllMail(0)
	for _, m := range mail {
		if m.Agent == "user" {
			if err := ps.MarkMailRead(m.ID); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if _, unread, userUnread, _, _ = s.MailTallies(); unread != 3 || userUnread != 1 {
		t.Errorf("after reading one of the user's: unread %d, user %d; want 3 and 1", unread, userUnread)
	}
}
