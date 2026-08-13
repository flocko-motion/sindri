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
	m, err := ps.AddMail("dvalin", "reviewer", "rejected: the gate is missing", true)
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

// TestMailIsFleetWideAndNewestFirst: the view spans agents and repos, which is what makes it a Mail
// section rather than a second per-agent timeline.
func TestMailIsFleetWideAndNewestFirst(t *testing.T) {
	s := mailStore(t)
	if _, err := s.For("one").AddMail("dvalin", "hub", "first", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.For("two").AddMail("nori", "user", "second", false); err != nil {
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
		if _, err := ps.AddMail("dvalin", "hub", strings.Repeat("x", i+1), false); err != nil {
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
	total, unread, byProject, err := s.MailTallies()
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || unread != 5 || byProject["proj"] != 5 {
		t.Errorf("tallies count the whole mailbox, got total=%d unread=%d byProject=%v", total, unread, byProject)
	}
	if err := ps.MarkMailRead(window[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, unread, byProject, _ = s.MailTallies(); unread != 4 || byProject["proj"] != 4 {
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
	if _, err := s.For("proj").AddMail("dvalin", "hub", "merged pr-sd-1", true); err != nil {
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
