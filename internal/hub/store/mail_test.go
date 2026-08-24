package store

import (
	"path/filepath"
	"strings"
	"testing"
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

// TestUnannouncedMailSkipsALandedPush: a message delivered as mail-and-push already reached the
// agent's pane, so counting it toward "unannounced" would nudge a second notification about
// something already sitting there — pushed and notified are two different questions, and either
// one answering "yes" is enough to skip it.
func TestUnannouncedMailSkipsALandedPush(t *testing.T) {
	s := mailStore(t)
	ps := s.For("proj")
	if _, err := ps.AddMail("dvalin", "hub", "already pushed", true, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddMail("dvalin", "hub", "never pushed", false, 0); err != nil {
		t.Fatal(err)
	}
	unannounced, unread, err := ps.UnannouncedMail("dvalin")
	if err != nil {
		t.Fatal(err)
	}
	if unread != 2 {
		t.Errorf("unread = %d, want 2 — both messages are still unread", unread)
	}
	if unannounced != 1 {
		t.Errorf("unannounced = %d, want 1 — the pushed one already reached the pane", unannounced)
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

// TestMailWindowKeepsTheRecentEnd: the window bounds the RENDER, not the record, so it has to keep
// the newest — and the tallies must still count everything, or a view would present its window as
// the history and the badge would stop rising once the mailbox outgrew it.
func TestMailWindowKeepsTheRecentEnd(t *testing.T) {
	s := mailStore(t)
	ps := s.For("proj")
	for i := 0; i < 5; i++ {
		if _, err := ps.AddMail("dvalin", "hub", strings.Repeat("x", i+1), false, 0); err != nil {
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
	if total != 5 || unread != 5 || byProject["proj"] != 5 {
		t.Errorf("tallies count the whole mailbox, got total=%d unread=%d byProject=%v", total, unread, byProject)
	}
	// None of it is addressed to the user, and that share comes out of the same single pass.
	if userUnread != 0 {
		t.Errorf("the user's own unread = %d, want 0 — this is all agent traffic", userUnread)
	}
	if err := ps.MarkMailRead(window[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, unread, _, byProject, _ = s.MailTallies(); unread != 4 || byProject["proj"] != 4 {
		t.Errorf("reading one should leave 4 unread, got %d / %v", unread, byProject)
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
