package cli

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// mailBoard is a two-message mailbox on a board whose window is smaller than the mailbox.
func mailBoard() api.BoardState {
	return api.BoardState{
		Mail: []api.Mail{
			{ID: 3, Repo: "one", Agent: "dvalin", Sender: "reviewer", Body: "rejected: the gate is missing", Pushed: true},
			{ID: 2, Repo: "one", Agent: "nori", Sender: "hub", Body: "merged pr-sd-1", ReadAt: "2026-08-13T09:30:00Z"},
		},
		MailTotal: 500, MailUnread: 120,
	}
}

// TestMailLineSaysWhoWhenAndWhetherRead is the CLI half of the same row the TUI draws: the recipient,
// what sent it, its read state — and whether it was pushed too, which is the difference between "it
// may have acted on this already" and "nothing has reached it".
func TestMailLineSaysWhoWhenAndWhetherRead(t *testing.T) {
	rows := mailBoard().Mail
	unread := mailLine(rows[0])
	for _, want := range []string{"dvalin", "reviewer", "unread", "pushed", "rejected: the gate is missing"} {
		if !strings.Contains(unread, want) {
			t.Errorf("the row should carry %q: %s", want, unread)
		}
	}
	read := mailLine(rows[1])
	if !strings.Contains(read, "read") || strings.Contains(read, "unread") {
		t.Errorf("a read message should say read, not unread: %s", read)
	}
}

// TestTheMailFooterSaysWhatItIsNotShowing: the listing is a window over a mailbox nothing is deleted
// from, so it has to disclose the window rather than let the recent end read as the whole history.
func TestTheMailFooterSaysWhatItIsNotShowing(t *testing.T) {
	st := mailBoard()
	got := mailFooter(st, st.Mail, api.MailAll, "")
	for _, want := range []string{"Showing the last 2 of 500", "120 unread", "mail show"} {
		if !strings.Contains(got, want) {
			t.Errorf("the footer should say %q: %s", want, got)
		}
	}
	// With the whole mailbox in hand there is nothing to disclose, and it says so plainly instead.
	st.MailTotal = 2
	if got := mailFooter(st, st.Mail, api.MailAll, ""); strings.Contains(got, "Showing the last") {
		t.Errorf("nothing is being withheld here: %s", got)
	}
	// A narrowing is named, so a short list never reads as an empty mailbox.
	if got := mailFooter(st, st.Mail[:1], api.MailUnread, "dvalin"); !strings.Contains(got, "to dvalin") {
		t.Errorf("the footer should name the recipient it was narrowed to: %s", got)
	}
	// And an empty mailbox says what the mailbox IS, since a bare "0 messages" reads as a fault.
	empty := api.BoardState{}
	if got := mailFooter(empty, nil, api.MailUnread, ""); !strings.Contains(got, "no mail yet") {
		t.Errorf("an empty mailbox should explain itself: %s", got)
	}
}

// TestTheCLIAndTUIFilterTheSameSet: both front-ends narrow through api.FilterMail, so a flag and a
// keypress cannot disagree about what "unread" admits.
func TestTheCLIAndTUIFilterTheSameSet(t *testing.T) {
	st := mailBoard()
	if got := api.FilterMail(api.MailUnread, "", st.Mail); len(got) != 1 || got[0].Agent != "dvalin" {
		t.Errorf("unread should keep only dvalin's message, got %+v", got)
	}
	if _, err := api.ParseMailFilter("unred"); err == nil {
		t.Error("a mistyped --filter must be refused rather than showing everything silently")
	}
}
