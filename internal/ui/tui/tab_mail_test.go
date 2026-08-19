package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// mailModel is the Mail tab over a two-agent mailbox, one message read, showing everything.
func mailModel() model {
	m := newModel(nil, nil, "/r/one")
	m.tab, m.scopeRepo, m.mailFilter = 6, false, api.MailAll // Mail is the seventh tab (Runs took the sixth)
	m.state = api.BoardState{
		Projects: []api.Project{{Tag: "repo", Path: "/r/one"}},
		Agents:   []api.AgentView{{Name: "dvalin", Project: "repo", Status: "idle"}},
		Mail: []api.Mail{
			{ID: 3, Project: "repo", Repo: "one", Agent: "dvalin", Sender: "reviewer",
				Body: "rejected: the gate is missing\nand the test is thin", Truncated: true, SentAt: "2026-08-13T10:00:00Z", Pushed: true},
			{ID: 2, Project: "repo", Repo: "one", Agent: "nori", Sender: "hub",
				Body: "merged pr-sd-1", SentAt: "2026-08-13T09:00:00Z", ReadAt: "2026-08-13T09:30:00Z"},
		},
		MailTotal: 2, MailUnread: 1, MailUnreadByRepo: map[string]int{"repo": 1},
	}
	m.reclamp()
	return m
}

// TestMailRowsSayWhoWhenAndWhetherRead is the subtask's DONE WHEN for a front-end: the recipient, what
// sent it, when, and its read state, with the body's opening — the whole of it belongs in the detail.
func TestMailRowsSayWhoWhenAndWhetherRead(t *testing.T) {
	m := mailModel()
	rows := strings.Join(rowTexts(m.mailRows()), "\n")
	for _, want := range []string{"dvalin", "reviewer", "unread", "nori", "hub", "read"} {
		if !strings.Contains(rows, want) {
			t.Errorf("the mail list should show %q:\n%s", want, rows)
		}
	}
	// A pushed message is marked as such: "pushed and possibly missed" and "sitting here unread"
	// are different diagnoses, and the row is where a reader tells them apart.
	if !strings.Contains(rows, "push") {
		t.Errorf("a message that was also injected should say so:\n%s", rows)
	}
	// One line per message, whatever the body does — the first row's body has a newline in it.
	if n := itemRows(m.mailRows()); n != 2 {
		t.Errorf("two messages should be two rows, got %d", n)
	}
}

// TestTheMailListSaysWhatItIsNotShowing: the board carries a window over a mailbox that is never
// pruned. A list that stopped at its rows would present the recent end as the whole history, and
// finding last month's message is the entire reason nothing is deleted.
func TestTheMailListSaysWhatItIsNotShowing(t *testing.T) {
	m := mailModel()
	if got := strings.Join(rowTexts(m.mailRows()), "\n"); strings.Contains(got, "showing the last") {
		t.Errorf("with the whole mailbox on the board there is nothing to disclose:\n%s", got)
	}
	m.state.MailTotal = 500 // the window is a fraction of the mailbox
	got := strings.Join(rowTexts(m.mailRows()), "\n")
	for _, want := range []string{"showing the last 2 of 500", "mail show"} {
		if !strings.Contains(got, want) {
			t.Errorf("the list should say %q:\n%s", want, got)
		}
	}
}

// TestTheMailDetailShowsTheMessageAndReachesItsAgent: a rejection with its feedback is exactly what is
// worth reading in full, so the detail carries the body — and the recipient is a cross-reference,
// since a message that looks wrong is a question about the agent it was sent to.
func TestTheMailDetailShowsTheMessageAndReachesItsAgent(t *testing.T) {
	m := mailModel()
	detail := strings.Join(m.mailDetailLines(), "\n")
	for _, want := range []string{"dvalin", "reviewer", "rejected: the gate is missing", "pushed:  true"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the detail should carry %q:\n%s", want, detail)
		}
	}
	// The board's row was a preview, so the detail says the rest is on its way rather than ending
	// mid-sentence as though that were the message.
	if !strings.Contains(detail, "fetching the rest") {
		t.Errorf("a truncated body should say it is truncated:\n%s", detail)
	}
	// Once the full body lands it replaces the preview outright.
	m.mailBody, m.mailBodyID = "rejected: the gate is missing\nand the test is thin\nand here is the rest", 3
	if detail := strings.Join(m.mailDetailLines(), "\n"); !strings.Contains(detail, "here is the rest") {
		t.Errorf("the fetched body should be shown whole:\n%s", detail)
	}
	var to metaItem
	for _, it := range m.mailItems() {
		if strings.HasPrefix(it.text, "to:") {
			to = it
		}
	}
	if to.kind != "agent" || to.value != "dvalin" {
		t.Errorf("the recipient should be a cross-reference to its agent, got kind=%q value=%q", to.kind, to.value)
	}
}

// TestMailFiltersNarrowTheList walks both axes from the keys that drive them, since a filter nobody
// can reach is not a filter. `f` cycles unread/all (the same set the CLI's --filter parses) and `w`
// narrows to the selected message's recipient.
func TestMailFiltersNarrowTheList(t *testing.T) {
	m := mailModel()
	m.mailFilter = api.MailUnread
	if n := itemRows(m.mailRows()); n != 1 {
		t.Errorf("unread should show one of the two messages, got %d", n)
	}
	m.onKey(keyFilter)
	if m.mailFilter != api.MailAll {
		t.Errorf("`%s` should cycle the mail filter, got %q", keyFilter, m.mailFilter)
	}
	if n := itemRows(m.mailRows()); n != 2 {
		t.Errorf("all should show both messages, got %d", n)
	}
	// `w` steps everyone → this row's recipient → YOU → everyone. The row step is first because the
	// cursor makes it obvious, and because after narrowing to the user no other recipient is left to
	// select — a row step placed last could never be reached.
	m.onKey(keyMailWho)
	if m.mailAgent != "dvalin" {
		t.Errorf("`%s` should narrow to the selected recipient, got %q", keyMailWho, m.mailAgent)
	}
	if n := itemRows(m.mailRows()); n != 1 {
		t.Errorf("narrowed to dvalin should show one message, got %d", n)
	}
	m.onKey(keyMailWho)
	if m.mailAgent != api.SenderUser {
		t.Errorf("`%s` should then narrow to the user, got %q", keyMailWho, m.mailAgent)
	}
	m.onKey(keyMailWho)
	if m.mailAgent != "" {
		t.Errorf("`%s` again should widen back to everyone, got %q", keyMailWho, m.mailAgent)
	}
}

// TestTheMailBadgeCountsUnreadNotTheWindow: the badge is read off the hub's tally of the whole
// mailbox, so it keeps rising when the history outgrows what the board carries — and it narrows with
// the § scope, like the other two tabs that toggle.
func TestTheMailBadgeCountsUnreadNotTheWindow(t *testing.T) {
	m := mailModel()
	m.state.MailUnread, m.state.MailUnreadByRepo = 42, map[string]int{"repo": 7}
	var mail tuiSection
	for _, s := range tuiSections {
		if s.Key == "mail" {
			mail = s
		}
	}
	if mail.Key == "" {
		t.Fatal("the TUI should have a Mail tab — the section model is what puts it there")
	}
	if got := m.tabCount(mail); got != 42 {
		t.Errorf("global scope should show the fleet's unread (42), got %d", got)
	}
	m.scopeRepo = true
	if got := m.tabCount(mail); got != 7 {
		t.Errorf("repo scope should show this repo's unread (7), got %d", got)
	}
}

// TestTheUsersMailIsShownFromEveryRepo: the marker beside the handle counts the user's unread
// fleet-wide, so a scope that hid the row it points at would say something waits and then show
// nothing. It is grouped under the to-you heading, above the rest of the mailbox, wherever it's from.
func TestTheUsersMailIsShownFromEveryRepo(t *testing.T) {
	m := mailModel()
	m.scopeRepo = true // narrowed to the repo in view
	m.state.Projects = []api.Project{{Tag: "repo", Path: "/r/one"}}
	m.state.Mail = append(m.state.Mail, api.Mail{
		ID: 9, Project: "elsewhere", Repo: "two", Agent: "user", Sender: "nori",
		Body: "the config field is documented backwards", SentAt: "2026-08-17T11:00:00Z",
	})
	rows := strings.Join(rowTexts(m.mailRows()), "\n")
	if !strings.Contains(rows, "documented backwards") {
		t.Errorf("a note to the user from another repo must still be listed:\n%s", rows)
	}
	if !strings.Contains(rows, api.MailToUserHeading(1)) {
		t.Errorf("and grouped under the to-you heading regardless of which repo it came from:\n%s", rows)
	}
	// It is marked as the user's own, since the list is mostly agent traffic.
	if !strings.Contains(rows, "→ you") {
		t.Errorf("the user's own rows should be marked:\n%s", rows)
	}
	// An agent's mail from another repo stays out: none of it is the user's to read.
	m.state.Mail = append(m.state.Mail, api.Mail{
		ID: 10, Project: "elsewhere", Repo: "two", Agent: "gloin", Sender: "hub", Body: "a verdict elsewhere",
	})
	if rows := strings.Join(rowTexts(m.mailRows()), "\n"); strings.Contains(rows, "a verdict elsewhere") {
		t.Errorf("agent traffic from another repo is not the user's business:\n%s", rows)
	}
}

// TestTheUserCanAlwaysAskWhatIsWaitingForThem is the gap the review found: the narrowing used to be
// reachable only by selecting a row already addressed to the user, so the question was unanswerable in
// the one case it matters — when nothing of theirs is on screen. Now it is a step of the cycle, so it
// is reachable whatever is selected, and immediate when nothing is.
func TestTheUserCanAlwaysAskWhatIsWaitingForThem(t *testing.T) {
	// Nothing at all to select: one press, straight to the question.
	empty := newModel(nil, nil, "")
	empty.tab, empty.scopeRepo, empty.mailFilter = 6, false, api.MailAll
	empty.reclamp()
	empty.onKey(keyMailWho)
	if empty.mailAgent != api.SenderUser {
		t.Errorf("with nothing selected the first step should be the user, got %q", empty.mailAgent)
	}

	// An agent's row selected and nothing of the user's in the list: still reachable, and the footer
	// names each state on the way so the next press is never a guess.
	m := newModel(nil, nil, "")
	m.tab, m.scopeRepo, m.mailFilter = 6, false, api.MailAll
	m.state = api.BoardState{Mail: []api.Mail{{ID: 2, Repo: "one", Agent: "dvalin", Sender: "hub", Body: "a verdict"}}}
	m.reclamp()
	for i := 0; i < len(api.MailFilters) && m.mailAgent != api.SenderUser; i++ {
		m.onKey(keyMailWho)
	}
	if m.mailAgent != api.SenderUser {
		t.Fatalf("the cycle should reach the user from any state, got %q", m.mailAgent)
	}
	// Empty of messages, but not of the row that says why: a narrowed list still states its filter
	// line (-> filterline.go listing), so "nothing addressed to them" reads as a narrowing the user
	// can clear rather than as a mailbox that has gone silent.
	if rows := m.mailRows(); len(rows) != 1 {
		t.Errorf("with nothing addressed to them the list holds only its filter line, not the agent's mail: %d rows", len(rows))
	}
	if got := mailWhoLabel(m.mailAgent); got != "you" {
		t.Errorf("the footer should say who it is narrowed to, got %q", got)
	}
}
