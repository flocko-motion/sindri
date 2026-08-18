package api

import (
	"strings"
	"testing"
	"time"
)

// fleetMail is a mailbox spanning two agents, one message read.
func fleetMail() []Mail {
	return []Mail{
		{ID: 3, Agent: "dvalin", Sender: "reviewer", Body: "rejected"},
		{ID: 2, Agent: "nori", Sender: "hub", Body: "merged", ReadAt: "2026-08-13T10:00:00Z"},
		{ID: 1, Agent: "dvalin", Sender: "user", Body: "have a look at this"},
	}
}

// TestMailFiltersNarrowOnBothAxes: unread-or-all and one recipient are independent, and a view
// applies both — "what has this agent not read yet" is the question the tab is opened with.
func TestMailFiltersNarrowOnBothAxes(t *testing.T) {
	all := fleetMail()
	if got := FilterMail(MailAll, "", all); len(got) != 3 {
		t.Errorf("all/every agent should keep everything, got %d", len(got))
	}
	if got := FilterMail(MailUnread, "", all); len(got) != 2 {
		t.Errorf("unread should drop the read one, got %d", len(got))
	}
	if got := FilterMail(MailAll, "nori", all); len(got) != 1 || got[0].Agent != "nori" {
		t.Errorf("an agent narrowing should keep only its mail, got %+v", got)
	}
	if got := FilterMail(MailUnread, "nori", all); len(got) != 0 {
		t.Errorf("nori's only message is read, so unread+nori is empty, got %+v", got)
	}
}

// TestAnUnknownFilterShowsEverything: a listing that showed nothing would read as an empty mailbox,
// which is a lie a mistyped flag must not be able to tell.
func TestAnUnknownFilterShowsEverything(t *testing.T) {
	if got := FilterMail(MailFilter("nonsense"), "", fleetMail()); len(got) != 3 {
		t.Errorf("an unrecognised filter admits everything, got %d", len(got))
	}
}

// TestMailFilterParsesAndDocumentsItselfFromOneSource is the pattern sd-c8681f established: the CLI's
// flag values, its help text and the TUI's cycle all come off MailFilters, so they cannot drift.
func TestMailFilterParsesAndDocumentsItselfFromOneSource(t *testing.T) {
	for _, f := range MailFilters {
		got, err := ParseMailFilter(string(f))
		if err != nil || got != f {
			t.Errorf("ParseMailFilter(%q) = %q, %v", f, got, err)
		}
	}
	if _, err := ParseMailFilter("nope"); err == nil {
		t.Error("an unknown name must be an error, so a mistyped flag says so")
	} else if names := MailFilterNames(); !contains(err.Error(), names) {
		t.Errorf("the error should name the whole set (%q): %v", names, err)
	}
	// The cycle covers the set and returns to its start, and an unknown filter lands inside it.
	seen := map[MailFilter]bool{}
	f := MailFilters[0]
	for range MailFilters {
		seen[f] = true
		f = NextMailFilter(f)
	}
	if len(seen) != len(MailFilters) || f != MailFilters[0] {
		t.Errorf("the cycle should walk every filter and wrap, saw %v ending on %q", seen, f)
	}
	if got := NextMailFilter("nonsense"); got != MailFilters[0] {
		t.Errorf("an unknown filter should land on the first, got %q", got)
	}
}

// TestUnreadIsCountedOverWhatItIsGiven, the badge's arithmetic.
func TestUnreadIsCountedOverWhatItIsGiven(t *testing.T) {
	if got := CountUnreadMail(fleetMail()); got != 2 {
		t.Errorf("CountUnreadMail = %d, want 2", got)
	}
	if got := CountUnreadMail(nil); got != 0 {
		t.Errorf("an empty mailbox has nothing unread, got %d", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestActiveIsUnreadPlusJustRead is the segment's whole shape, and the reason it is the default: the
// mailbox is never pruned, so "all" grows for the life of the machine while "unread" hides what you
// have just dealt with. Active is the union, exactly as it is for tasks.
func TestActiveIsUnreadPlusJustRead(t *testing.T) {
	now := time.Now().UTC()
	recent := now.Add(-time.Minute).Format(time.RFC3339)
	old := now.Add(-48 * time.Hour).Format(time.RFC3339)

	unread := Mail{ID: 1, Agent: "dvalin", SentAt: old}
	justRead := Mail{ID: 2, Agent: "dvalin", SentAt: old, ReadAt: recent}
	longRead := Mail{ID: 3, Agent: "dvalin", SentAt: old, ReadAt: old}

	got := FilterMail(MailActive, "", []Mail{unread, justRead, longRead})
	if len(got) != 2 {
		t.Fatalf("active should keep the unread and the just-read, got %d", len(got))
	}
	for _, m := range got {
		if m.ID == 3 {
			t.Error("a message read two days ago is not active")
		}
	}
	// The change time is the READ, not the send: this message was sent days ago and dealt with now.
	if MailChangedAt(justRead) != recent {
		t.Errorf("MailChangedAt = %q, want the read time %q", MailChangedAt(justRead), recent)
	}
	if MailChangedAt(unread) != old {
		t.Errorf("an unread message last changed when it was sent, got %q", MailChangedAt(unread))
	}
}

// TestActiveLeadsAndUnreadRemains: active is what a view opens on, and the cycle reaches it first —
// but unread stays, being the sharpest question to ask of a mailbox.
func TestActiveLeadsAndUnreadRemains(t *testing.T) {
	if MailFilters[0] != MailActive {
		t.Errorf("MailFilters should lead with active, got %q", MailFilters[0])
	}
	found := false
	for _, f := range MailFilters {
		if f == MailUnread {
			found = true
		}
	}
	if !found {
		t.Error("unread must remain on offer")
	}
	if got := NextMailFilter(MailActive); got != MailUnread {
		t.Errorf("the cycle should go active → unread, got %q", got)
	}
}

// TestTheBadgeStillCountsUnreadNotActive is the invariant the PR badge already holds: the marker says
// what NEEDS reading, and a message read ten minutes ago needs nothing.
func TestTheBadgeStillCountsUnreadNotActive(t *testing.T) {
	recent := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	mail := []Mail{
		{ID: 1, Agent: "user", SentAt: recent},
		{ID: 2, Agent: "user", SentAt: recent, ReadAt: recent},
	}
	if got := CountUnreadMail(mail); got != 1 {
		t.Errorf("CountUnreadMail = %d, want 1 — the badge must not follow the filter", got)
	}
}

// TestMailIdsReadAsIdsAndBothSpellingsParse: every other id in sindri carries a prefix, and a bare
// integer beside agent names and ages does not read as something you can address. The bare form still
// parses, because it is already in shell history and in whatever agents have been told.
func TestMailIdsReadAsIdsAndBothSpellingsParse(t *testing.T) {
	if got := MailID(47); got != "ml-47" {
		t.Errorf("MailID(47) = %q, want ml-47", got)
	}
	for _, in := range []string{"ml-47", "47", " ml-47 "} {
		got, err := ParseMailID(in)
		if err != nil || got != 47 {
			t.Errorf("ParseMailID(%q) = %d, %v; want 47", in, got, err)
		}
	}
	// A round trip, since the rendered form is what a front-end carries as a row id and then parses back.
	if got, err := ParseMailID(MailID(1234)); err != nil || got != 1234 {
		t.Errorf("round trip failed: %d, %v", got, err)
	}
	for _, bad := range []string{"", "ml-", "ml-x", "no", "0", "-3"} {
		if _, err := ParseMailID(bad); err == nil {
			t.Errorf("ParseMailID(%q) should refuse rather than resolve to a mailbox", bad)
		}
	}
	// The refusal shows the shape, since somebody typing a wrong one has not seen a right one.
	if _, err := ParseMailID("nonsense"); err == nil || !strings.Contains(err.Error(), MailIDPrefix) {
		t.Errorf("the refusal should show the shape: %v", err)
	}
}
