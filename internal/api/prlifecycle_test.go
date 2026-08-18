package api

import "testing"

// prHistory is a PR's log as the hub writes it: the milestones interleaved with the diagnostics
// that make the full block unreadable.
func prHistory() []Event {
	return []Event{
		{TS: "2026-08-13T09:00:00Z", Type: "created", Payload: "by bombur: add the widget"},
		{TS: "2026-08-13T09:01:00Z", Type: "review-requested", Payload: "assigned to fili"},
		{TS: "2026-08-13T09:02:00Z", Type: "precheck-pass", Payload: "applies onto main and the gate passes"},
		{TS: "2026-08-13T09:10:00Z", Type: "rejected", Payload: "by fili: the gate fails"},
		{TS: "2026-08-13T09:20:00Z", Type: "resubmitted", Payload: "by bombur: fixed the gate"},
		{TS: "2026-08-13T09:25:00Z", Type: "conflict", Payload: "rebase onto main conflicts: a.go"},
		{TS: "2026-08-13T09:30:00Z", Type: "approved", Payload: "by fili"},
		{TS: "2026-08-13T09:40:00Z", Type: "merged", Payload: "into main"},
	}
}

// TestLifecycleKeepsTheStoryAndDropsTheDiagnostics is the whole point: the summary answers "where
// is this up to", and the events that answer "why did it go wrong" stay in the log below it.
func TestLifecycleKeepsTheStoryAndDropsTheDiagnostics(t *testing.T) {
	got := PRLifecycle(prHistory(), nil)
	want := []string{"created", "rejected", "resubmitted", "approved", "merged"}
	if len(got) != len(want) {
		t.Fatalf("lifecycle = %d entries %v, want %d", len(got), got, len(want))
	}
	for i, w := range want {
		if got[i].Event != w {
			t.Errorf("entry %d = %q, want %q — and in the order they happened", i, got[i].Event, w)
		}
	}
	if got[0].Who != "bombur" || got[0].Note != "add the widget" {
		t.Errorf("created = %+v, want the author and the message split apart", got[0])
	}
	if got[4].At != "2026-08-13T09:40:00Z" {
		t.Errorf("merged carries the wrong time: %+v", got[4])
	}
}

// TestVerdictsComeFromTheReviewRecords: the payload says "by fili" as prose, the review record
// holds it as data along with the moment the verdict was given — so the record wins, and two
// verdicts read as two authors rather than both naming the first.
func TestVerdictsComeFromTheReviewRecords(t *testing.T) {
	reviews := []Review{
		{Author: "fili", Verdict: "changes", VerdictAt: "2026-08-13T09:11:00Z"},
		{Author: "kili", Verdict: "pass", VerdictAt: "2026-08-13T09:31:00Z", Advisory: true},
	}
	got := PRLifecycle(prHistory(), reviews)
	var rejected, approved PRMilestone
	for _, ms := range got {
		switch ms.Event {
		case "rejected":
			rejected = ms
		case "approved":
			approved = ms
		}
	}
	if rejected.Who != "fili" || rejected.At != "2026-08-13T09:11:00Z" {
		t.Errorf("rejected = %+v, want fili at the verdict's own time", rejected)
	}
	if approved.Who != "kili (advisory)" {
		t.Errorf("approved = %+v, want the second verdict's author, marked advisory", approved)
	}
	// With no record behind it, the event keeps what its payload said rather than losing the name.
	if plain := PRLifecycle(prHistory(), nil); plain[3].Who != "fili" {
		t.Errorf("approved without a review record = %+v, want the payload's author", plain[3])
	}
}

// TestAnUnknownEventIsShownNotHidden: the build fails on an unclassified type (-> hub's
// TestEveryPREventIsClassified), and until someone fixes it the summary shows the stranger. A
// hidden state is invisible; a noisy line gets classified.
func TestAnUnknownEventIsShownNotHidden(t *testing.T) {
	milestone, known := PREventKind("teleported")
	if known {
		t.Fatal("the vocabulary should not know an invented event")
	}
	if !milestone {
		t.Error("an unclassified event must surface, or a new state can go unseen for ever")
	}
	if _, known := PREventKind("merged"); !known {
		t.Error("a real event type must be known")
	}
}

// TestSplitActorReadsThePayloadsThatExist walks the shapes the hub actually writes, since a wrong
// split puts someone else's name against an event.
func TestSplitActorReadsThePayloadsThatExist(t *testing.T) {
	cases := []struct{ payload, who, note string }{
		{"by bombur: add the widget", "bombur", "add the widget"},
		{"by user", "user", ""},
		{"milestone by dvalin", "dvalin", "milestone"},
		{"interim, by bombur: partial work", "bombur", "interim, partial work"},
		{"into main", "", "into main"},
		{"", "", ""},
	}
	for _, c := range cases {
		who, note := splitActor(c.payload)
		if who != c.who || note != c.note {
			t.Errorf("splitActor(%q) = (%q, %q), want (%q, %q)", c.payload, who, note, c.who, c.note)
		}
	}
}
