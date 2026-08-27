package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestTheBoardCarriesAWindowAndCountsTheWholeMailbox: the mailbox is never pruned, so what has to be
// bounded is the render — and only the READ end of it, so a badge never counts a message no list can
// reach (sd-ca8929). The tallies beside the window count EVERYTHING, which is what lets a view say
// "showing the last N of M" instead of presenting its window as the history.
func TestTheBoardCarriesAWindowAndCountsTheWholeMailbox(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	const over = MailWindow + 6
	var oldestUnread AgentMail
	for i := 0; i < over; i++ {
		m, err := ps.AddMail("dvalin", "hub", "message", false, 0)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			oldestUnread = m // stays unread — older than a read-only window could ever reach
			continue
		}
		if err := ps.MarkMailRead(m.ID); err != nil {
			t.Fatal(err)
		}
	}
	window, total, unread, userUnread, byRepo, err := h.mailWindow()
	if err != nil {
		t.Fatal(err)
	}
	if len(window) != MailWindow {
		t.Errorf("the window should hold %d messages, got %d", MailWindow, len(window))
	}
	found := false
	for _, m := range window {
		if m.ID == oldestUnread.ID {
			found = true
		}
	}
	if !found {
		t.Error("the one unread message must ride in the window despite being the oldest of all")
	}
	if total != over || unread != 1 {
		t.Errorf("the tallies count the whole mailbox, got total=%d unread=%d, want %d and 1", total, unread, over)
	}
	if userUnread != 0 {
		t.Errorf("none of this is addressed to the user, so its own tally is %d, want 0", userUnread)
	}
	if byRepo[testProject] != 1 {
		t.Errorf("unread per repo = %v, want 1 for %s", byRepo, testProject)
	}
	// Each row carries its repo, resolved by the hub, so a front-end renders rather than resolves it.
	if window[0].Repo == "" {
		t.Errorf("a row should carry its repo name: %+v", window[0])
	}
}

// TestTheBadgeAndTheWindowAgreeByConstruction pins sd-ca8929's actual fix, not just its repro: since
// every unread message rides in the window regardless of age, the fleet-wide unread tally and a plain
// count over the board's own Mail slice must always match — no second rule needed to keep them in
// step, and none left to drift.
func TestTheBadgeAndTheWindowAgreeByConstruction(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	const over = MailWindow + 10
	for i := 0; i < over; i++ {
		m, err := ps.AddMail("dvalin", "hub", "message", false, 0)
		if err != nil {
			t.Fatal(err)
		}
		// Every third message stays unread, scattered across the whole age range rather than bunched
		// at one end — the shape most likely to expose a window that only fixed one of them.
		if i%3 != 0 {
			if err := ps.MarkMailRead(m.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	board, err := h.State("")
	if err != nil {
		t.Fatal(err)
	}
	if got := api.CountUnreadMail(board.Mail); got != board.MailUnread {
		t.Errorf("the window's own unread count = %d, the badge = %d — they must agree", got, board.MailUnread)
	}
}

// TestALongBodyIsPreviewedInTheWindowAndWholeByID: a rejection arrives with its whole findings, which
// can run to hundreds of lines. The list carries an opening and SAYS it was cut — a tail mistaken for
// the end of a message is the one way this view could mislead — and the detail fetches it in full.
func TestALongBodyIsPreviewedInTheWindowAndWholeByID(t *testing.T) {
	h := newHub(t)
	long := strings.Repeat("finding. ", 200)
	m, err := h.store.For(testProject).AddMail("dvalin", "reviewer", long, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	window, _, _, _, _, err := h.mailWindow()
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
	read, err := ps.AddMail("dvalin", "hub", "merged pr-sd-1", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddMail("dvalin", "reviewer", "rejected", true, 0); err != nil {
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

// TestTheMarkerCountsOnlyTheUsersUnread is the read half's whole point: the mailbox is mostly agent
// traffic, none of it a person's to read, so a marker over all of it would be permanently lit and
// instantly ignored. It counts what is addressed to the user, and counts it fleet-wide.
func TestTheMarkerCountsOnlyTheUsersUnread(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	// The bulk: hub-to-agent and agent-to-agent, unread, and none of it the user's.
	for i := 0; i < 5; i++ {
		if _, err := ps.AddMail("dvalin", "hub", "a verdict", true, 0); err != nil {
			t.Fatal(err)
		}
	}
	// Two notes to the user, one already read.
	read, err := ps.AddMail("user", "dvalin", "the td adapter shells out twice", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddMail("user", "nori", "config field is documented backwards", false, 0); err != nil {
		t.Fatal(err)
	}
	if err := ps.MarkMailRead(read.ID); err != nil {
		t.Fatal(err)
	}

	board, err := h.State("")
	if err != nil {
		t.Fatal(err)
	}
	if board.MailUnreadUser != 1 {
		t.Errorf("the user's unread = %d, want 1 of the seven", board.MailUnreadUser)
	}
	if board.MailUnread != 6 {
		t.Errorf("the mailbox's own unread = %d, want 6 — the two numbers answer different questions", board.MailUnread)
	}
	if got := board.SectionAttention("mail"); got != 1 {
		t.Errorf("the Mail marker = %d, want the user's own unread (1)", got)
	}
}

// TestMailAttentionCountsExactlyUnreadAndToTheUser pins the whole 2x2 the marker's conjunction rests
// on, one cell per case rather than one mailbox for all four — a test that only asserts the cell
// that already works would have missed sd-ac1757's single dropped conjunct.
func TestMailAttentionCountsExactlyUnreadAndToTheUser(t *testing.T) {
	cases := []struct {
		name  string
		agent string
		read  bool
		want  int
	}{
		{"to the user, unread", "user", false, 1},
		{"to the user, read", "user", true, 0},
		{"to an agent, unread", "dvalin", false, 0},
		{"to an agent, read", "dvalin", true, 0},
	}
	for _, c := range cases {
		h := newHub(t)
		ps := h.store.For(testProject)
		m, err := ps.AddMail(c.agent, "hub", "a message", false, 0)
		if err != nil {
			t.Fatal(err)
		}
		if c.read {
			if err := ps.MarkMailRead(m.ID); err != nil {
				t.Fatal(err)
			}
		}
		board, err := h.State("")
		if err != nil {
			t.Fatal(err)
		}
		if board.MailUnreadUser != c.want {
			t.Errorf("%s: MailUnreadUser = %d, want %d", c.name, board.MailUnreadUser, c.want)
		}
		if got := board.SectionAttention("mail"); got != c.want {
			t.Errorf("%s: mail section attention = %d, want %d", c.name, got, c.want)
		}
	}
}

// TestMarkMailReadForUserNotifiesAtOnce: the agent read path calls notify explicitly ("the unread
// count is on the board, and it has just changed" — mailverb.go); the user's own path has to too, or
// the marker only falls on the next poll rather than the instant it is read.
func TestMarkMailReadForUserNotifiesAtOnce(t *testing.T) {
	h := newHub(t)
	m, err := h.store.For(testProject).AddMail("user", "dvalin", "a note", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	ch, unsub := h.events.subscribe()
	defer unsub()
	if err := h.MarkMailReadForUser(m.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
	default:
		t.Error("MarkMailReadForUser should notify subscribers so the marker falls at once, not on the next poll")
	}
}
