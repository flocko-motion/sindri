package hub

import (
	"strings"
	"testing"
)

// TestTheBoardCarriesAWindowAndCountsTheWholeMailbox: the mailbox is never pruned, so what has to be
// bounded is the render. The board carries the newest MailWindow messages — and the tallies beside
// them count EVERYTHING, which is what lets a view say "showing the last N of M" instead of
// presenting its window as the history.
func TestTheBoardCarriesAWindowAndCountsTheWholeMailbox(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	const over = MailWindow + 6
	for i := 0; i < over; i++ {
		if _, err := ps.AddMail("dvalin", "hub", "message", false); err != nil {
			t.Fatal(err)
		}
	}
	window, total, unread, byRepo, err := h.mailWindow()
	if err != nil {
		t.Fatal(err)
	}
	if len(window) != MailWindow {
		t.Errorf("the window should hold %d messages, got %d", MailWindow, len(window))
	}
	if total != over || unread != over {
		t.Errorf("the tallies count the whole mailbox, got total=%d unread=%d, want %d", total, unread, over)
	}
	if byRepo[testProject] != over {
		t.Errorf("unread per repo = %v, want %d for %s", byRepo, over, testProject)
	}
	// Each row carries its repo, resolved by the hub, so a front-end renders rather than resolves it.
	if window[0].Repo == "" {
		t.Errorf("a row should carry its repo name: %+v", window[0])
	}
}

// TestALongBodyIsPreviewedInTheWindowAndWholeByID: a rejection arrives with its whole findings, which
// can run to hundreds of lines. The list carries an opening and SAYS it was cut — a tail mistaken for
// the end of a message is the one way this view could mislead — and the detail fetches it in full.
func TestALongBodyIsPreviewedInTheWindowAndWholeByID(t *testing.T) {
	h := newHub(t)
	long := strings.Repeat("finding. ", 200)
	m, err := h.store.For(testProject).AddMail("dvalin", "reviewer", long, true)
	if err != nil {
		t.Fatal(err)
	}
	window, _, _, _, err := h.mailWindow()
	if err != nil {
		t.Fatal(err)
	}
	if len(window) != 1 {
		t.Fatalf("expected the one message in the window, got %d", len(window))
	}
	if !window[0].Truncated || len(window[0].Body) >= len(long) {
		t.Errorf("the window should carry a marked preview, got truncated=%v len=%d of %d",
			window[0].Truncated, len(window[0].Body), len(long))
	}
	if !strings.HasPrefix(long, window[0].Body) {
		t.Error("the preview should be the opening of the body, not a rewrite of it")
	}
	got, ok, err := h.MailBody(m.ID)
	if err != nil || !ok {
		t.Fatalf("MailBody(%d): ok=%v err=%v", m.ID, ok, err)
	}
	if got.Body != long || got.Truncated {
		t.Errorf("MailBody must return the whole message uncut, got %d of %d chars (truncated=%v)",
			len(got.Body), len(long), got.Truncated)
	}
}

// TestTheMailSectionCountsUnread ties the store's tally to the badge every front-end draws, through
// the board the hub actually assembles.
func TestTheMailSectionCountsUnread(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	read, err := ps.AddMail("dvalin", "hub", "merged pr-sd-1", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddMail("dvalin", "reviewer", "rejected", true); err != nil {
		t.Fatal(err)
	}
	if err := ps.MarkMailRead(read.ID); err != nil {
		t.Fatal(err)
	}
	board, err := h.State("")
	if err != nil {
		t.Fatal(err)
	}
	if board.MailTotal != 2 || board.MailUnread != 1 {
		t.Errorf("board mail: total=%d unread=%d, want 2 and 1", board.MailTotal, board.MailUnread)
	}
	if got := board.SectionAttention("mail"); got != 0 {
		t.Errorf("mail asks nothing of the user, so its attention count is 0, got %d", got)
	}
	count, found := 0, false
	for _, s := range board.Sections {
		if s.Key == "mail" {
			count, found = s.Count, true
		}
	}
	if !found {
		t.Fatal("the board must carry a Mail section — that is how both front-ends pick the tab up")
	}
	if count != 1 {
		t.Errorf("the Mail badge is unread, got %d, want 1", count)
	}
}
