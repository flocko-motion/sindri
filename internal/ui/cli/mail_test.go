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
	for _, want := range []string{"Showing 2 of 500", "every unread one included", "120 unread", "mail show"} {
		if !strings.Contains(got, want) {
			t.Errorf("the footer should say %q: %s", want, got)
		}
	}
	// With the whole mailbox in hand there is nothing to disclose, and it says so plainly instead.
	st.MailTotal = 2
	if got := mailFooter(st, st.Mail, api.MailAll, ""); strings.Contains(got, "Showing") {
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

// TestPushAndMailReadAsDifferentActions is the subtask's legibility requirement, checked where a user
// actually meets it: the two one-liners. Someone choosing between them must see which interrupts and
// which waits without opening any documentation.
func TestPushAndMailReadAsDifferentActions(t *testing.T) {
	tell, mail := agentTellCmd().Short, agentMailCmd().Short
	if tell == mail {
		t.Fatal("the two actions must not describe themselves the same way")
	}
	for _, want := range []string{"now", "lost"} {
		if !strings.Contains(strings.ToLower(tell), want) {
			t.Errorf("tell's one-liner should say it %s: %q", want, tell)
		}
	}
	for _, want := range []string{"waits", "never interrupts"} {
		if !strings.Contains(strings.ToLower(mail), want) {
			t.Errorf("mail's one-liner should say it %s: %q", want, mail)
		}
	}
	// And each is its own subcommand, not a flag on the other — the model is two actions.
	if agentMailCmd().Name() != "mail" || agentTellCmd().Name() != "tell" {
		t.Errorf("expected two named actions, got %q and %q", agentTellCmd().Name(), agentMailCmd().Name())
	}
}

// TestMailShowStateNamesAFreshMark: `mail show` marks the user's own mail read on the spot, but the
// fetched value can't reflect a mark made after it was returned — justRead is how the print line
// says so anyway, rather than showing "unread" for a message it just retired.
func TestMailShowStateNamesAFreshMark(t *testing.T) {
	unread := api.Mail{}
	if got := mailShowState(unread, false); got != "unread" {
		t.Errorf("mailShowState(unread, false) = %q, want unread", got)
	}
	if got := mailShowState(unread, true); got != "read just now" {
		t.Errorf("mailShowState(unread, true) = %q, want \"read just now\"", got)
	}
	alreadyRead := api.Mail{ReadAt: "2026-08-13T09:00:00Z"}
	if got := mailShowState(alreadyRead, false); !strings.Contains(got, "ago") {
		t.Errorf("mailShowState(read, false) = %q, want it to say how long ago", got)
	}
}

// TestTheUsersOwnMailIsGroupedAndNamed is the parity half: both front-ends answer "is anything waiting
// for me?" the same way, so a note from another repo is grouped under the shared foreign heading rather
// than interleaved, and the closing line names how many are the user's.
func TestTheUsersOwnMailIsGroupedAndNamed(t *testing.T) {
	rows := []listRow{
		{line: "to you, elsewhere", group: listGroupFor("two", "one", true)},
		{line: "to an agent, here", group: listGroupFor("one", "one", false)},
		{line: "to an agent, elsewhere", group: listGroupFor("two", "one", false)},
	}
	got := strings.Join(groupedLines(rows), "\n")
	if !strings.Contains(got, api.ForeignAttentionHeading(1)) {
		t.Errorf("the user's note from another repo should sit under the foreign heading:\n%s", got)
	}
	if strings.Index(got, "to you, elsewhere") > strings.Index(got, "to an agent, here") {
		t.Errorf("what waits on the user comes first:\n%s", got)
	}
	for _, want := range []string{api.LocalHeading, api.OtherReposHeading} {
		if !strings.Contains(got, want) {
			t.Errorf("the other sections should be labelled too (%q):\n%s", want, got)
		}
	}
	st := api.BoardState{Mail: mailBoard().Mail, MailTotal: 3, MailUnread: 3, MailUnreadUser: 2}
	footer := mailFooter(st, st.Mail, api.MailUnread, "")
	if !strings.Contains(footer, "2 of them addressed to YOU") || !strings.Contains(footer, "--mine") {
		t.Errorf("the footer should name the user's own unread and how to narrow to it: %s", footer)
	}
}
