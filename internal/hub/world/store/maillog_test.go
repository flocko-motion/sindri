package store

import (
	"path/filepath"
	"testing"
)

// mailLogStore is a project store with one message in it, ready to have a lifecycle written.
func mailLogStore(t *testing.T) (*ProjectStore, int64) {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("proj", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	m, err := ps.AddMail("dvalin", "user", "pr-sd-1 was rejected", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	return ps, m.ID
}

// TestTheLifecycleRecordsWhatHappenedNotWhereItStands: the four flags on a mail row report the
// LATEST state, so a push that failed is indistinguishable from one never attempted, and the reason
// is nowhere. dvalin's rejection carried pushed=1 and never reached its pane.
func TestTheLifecycleRecordsWhatHappenedNotWhereItStands(t *testing.T) {
	ps, id := mailLogStore(t)

	if err := ps.LogMail(id, MailPushFailed, "no live session"); err != nil {
		t.Fatal(err)
	}
	if err := ps.LogMail(id, MailPushLanded, ""); err != nil {
		t.Fatal(err)
	}

	evs, err := ps.MailEvents(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("both attempts must survive, got %d", len(evs))
	}
	// Oldest first: the order it happened in is the whole point of keeping a trail.
	if evs[0].Type != MailPushFailed || evs[1].Type != MailPushLanded {
		t.Errorf("events out of order: %q then %q", evs[0].Type, evs[1].Type)
	}
	if evs[0].Payload != "no live session" {
		t.Errorf("the reason a push failed is the fact worth keeping, got %q", evs[0].Payload)
	}
}

// TestReadAndAnnounceAreRecordedOnce: re-reading changes nothing, and a trail that grew on every
// look would bury the delivery it exists to explain.
func TestReadAndAnnounceAreRecordedOnce(t *testing.T) {
	ps, id := mailLogStore(t)

	for i := 0; i < 3; i++ {
		if err := ps.MarkMailAnnounced("dvalin"); err != nil {
			t.Fatal(err)
		}
		if err := ps.MarkMailRead(id); err != nil {
			t.Fatal(err)
		}
	}

	counts := map[string]int{}
	evs, err := ps.MailEvents(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		counts[e.Type]++
	}
	if counts[MailAnnounced] != 1 {
		t.Errorf("announced recorded %d times, want once", counts[MailAnnounced])
	}
	if counts[MailRead] != 1 {
		t.Errorf("read recorded %d times, want once", counts[MailRead])
	}
}

// TestPushOnlyTrafficKeepsNoLifecycle: a wake carries no mail row, so there is nothing to hang a
// history on — and inventing one would file events against mail id 0 for every agent at once.
func TestPushOnlyTrafficKeepsNoLifecycle(t *testing.T) {
	ps, _ := mailLogStore(t)
	if err := ps.LogMail(0, MailPushLanded, ""); err != nil {
		t.Fatalf("logging against no message should be a no-op, got %v", err)
	}
	if evs, err := ps.MailEvents(0); err != nil || len(evs) != 0 {
		t.Errorf("mail 0 must carry nothing, got %d events (err %v)", len(evs), err)
	}
}
