package api

import "testing"

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
