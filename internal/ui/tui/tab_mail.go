// package: tui / mail
// type:    ui (Mail tab)
// job:     the Mail tab content — what agents have been sent and must read, fleet-wide and
// newest first, with the selected message's full body in the detail. The list is a
// window over a mailbox that is never pruned, so it says what it is not showing.
// limits:  renders the board's mail and fetches one body on selection; the filters are
// api's (-> api.MailFilters) and the record is the hub's.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/ui/table"
)

// mailVisible admits a message to the list: in scope OR addressed to the user, and admitted by the two
// filters. Mail to the user ignores the repo scope on purpose — the marker beside the handle counts it
// fleet-wide, so a scope that hid the row it points at would say something waits and show nothing. The
// row carries its own repo, which is what says it came from elsewhere.
func (m model) mailVisible(msg api.Mail) bool {
	if !api.MatchesMailFilter(m.mailFilter, m.mailAgent, msg) {
		return false
	}
	return m.inScope(msg.Project) || api.MailToUser(msg)
}

// mailShown is the board's mail window, filtered to what this view shows.
func (m model) mailShown() []api.Mail {
	var out []api.Mail
	for _, msg := range m.state.Mail {
		if m.mailVisible(msg) {
			out = append(out, msg)
		}
	}
	return out
}

// mailTable is the Mail list's columns. Sender BEFORE recipient, the order mail is read in
// everywhere else: adjacent and unlabelled, the reverse was misread over and over, and labelling a
// backwards order would only have made the backwardness legible.
var mailTable = table.Table{
	{Label: "repo", Width: 10, Clip: true},
	{Label: "from", Width: 10},
	{Label: "to", Width: 12},
	{Label: "state", Width: 12},
	{Label: "age", Width: 5, Right: true},
	{Label: "message"},
}

func (m model) mailRows() []row {
	var foreign, local []row
	for _, msg := range m.mailShown() {
		state, st := "unread", stWarn
		if msg.Read() {
			state, st = "read", stDone
		}
		if msg.Pushed { // also injected live, so it may already have been acted on
			state += "+push"
		}
		// The user's own mail is marked, not merely present: in a list mostly of agent traffic, the few
		// rows a person is expected to read have to be findable at a glance.
		if api.MailToUser(msg) && !msg.Read() {
			state = "→ you " + state
		}
		r := row{mailTable.Line(
			table.Cell{Text: msg.Repo, Style: m.repoStyle(msg.Project).Render},
			table.Cell{Text: dash(msg.Sender)},
			table.Cell{Text: msg.Agent},
			table.Cell{Text: state, Style: st.Render},
			table.Cell{Text: shortAge(msg.SentAt), Style: dimStyle.Render},
			table.Cell{Text: oneLineText(msg.Body)},
		), fmt.Sprint(msg.ID)}
		// Foreign here means the same as on the other scoped tabs: on screen only because it waits on
		// the user, so the heading says so — a repo column is skimmed (-> sectioned).
		if m.inScope(msg.Project) {
			local = append(local, r)
		} else {
			foreign = append(foreign, r)
		}
	}
	rows := listing(mailTable, foreign, local)
	// The window is not the history: a list that stopped at its rows would present the recent end as
	// everything, and finding last month's message is the whole reason nothing is deleted. Outside the
	// labelled rows, since it is a note about the listing rather than a message in it.
	if n, total := len(m.state.Mail), m.state.MailTotal; total > n {
		rows = append(rows, row{dimStyle.Render(fmt.Sprintf("… showing the last %d of %d messages — older mail: `sindri mail show <id>`", n, total)), ""})
	}
	return rows
}

// oneLineText is a body as a row shows it: its first line, since a message is prose and a row is a
// row. The whole of it is in the detail, which is where a long message is meant to be read.
func oneLineText(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimRight(s[:i], " ") + dimStyle.Render(" …")
	}
	return s
}

// selMail returns the selected message from the board's window.
func (m model) selMail() (api.Mail, bool) {
	id := m.selID()
	for _, msg := range m.state.Mail {
		if fmt.Sprint(msg.ID) == id {
			return msg, true
		}
	}
	return api.Mail{}, false
}

// mailItems is the selected message's fields plus its body. The recipient is a cross-reference: a
// message that looks wrong is a question about the agent it was sent to, so `g` goes there.
func (m model) mailItems() []metaItem {
	msg, ok := m.selMail()
	if !ok {
		return []metaItem{{text: dimStyle.Render("(no message selected)")}}
	}
	read := "unread"
	if msg.Read() {
		read = "read " + shortAge(msg.ReadAt) + " ago"
	}
	items := []metaItem{
		{text: "to:      " + msg.Agent + dimStyle.Render("  ("+msg.Repo+")"), kind: "agent", value: msg.Agent},
		{text: "from:    " + dash(msg.Sender)},
		{text: "sent:    " + msg.SentAt},
		{text: "state:   " + read},
		// Whether it was also pushed is the difference between "it may have acted on this already"
		// and "nothing has reached it yet", which is what a reader of a quiet agent is asking.
		{text: fmt.Sprintf("pushed:  %v", msg.Pushed)},
		{text: ""},
	}
	for _, line := range strings.Split(strings.TrimRight(m.mailBodyOf(msg), "\n"), "\n") {
		items = append(items, metaItem{text: line})
	}
	return items
}

// mailBodyOf is the message's text: the fetched full body once it has landed, else the preview the
// board carried — marked as cut, so a truncated tail never reads as the end of the message.
func (m model) mailBodyOf(msg api.Mail) string {
	if m.mailBodyID == msg.ID && m.mailBody != "" {
		return m.mailBody
	}
	if msg.Truncated {
		return msg.Body + dimStyle.Render("\n… (fetching the rest)")
	}
	return msg.Body
}

func (m model) mailDetailLines() []string { return itemTexts(mailWrapped(m)) }

// mailWrapped is the detail column's items, wrapped to its width — a body arrives as prose, so it
// wraps rather than being cut at the column edge.
func mailWrapped(m model) []metaItem { return wrapMeta(m.mailItems(), m.detailWidth()) }

// mailActionable is the focusable subset of the mail detail (the recipient cross-reference).
func (m model) mailActionable() []metaItem {
	var out []metaItem
	for _, it := range m.mailItems() {
		if it.kind != "" {
			out = append(out, it)
		}
	}
	return out
}

// mailBodyFetchCmd fetches one message in full. The board carries a preview per row, so the whole
// text — a rejection's findings run to hundreds of lines — is read only for what is being looked at.
func mailBodyFetchCmd(cl *client.HTTP, id int64) tea.Cmd {
	return func() tea.Msg {
		msg, err := cl.MailBody(id)
		if err != nil {
			return nil
		}
		return mailMsg{id: id, body: msg.Body}
	}
}

// cycleMailFilter steps the unread↔all cycle, shared with the CLI's --filter (-> api.MailFilters).
func (m *model) cycleMailFilter() {
	m.mailFilter = api.NextMailFilter(m.mailFilter)
	m.cursor[m.tab] = 0
	m.flash = "mail: " + string(m.mailFilter)
}

// toggleMailAgent narrows the list to the selected message's recipient, or clears that narrowing —
// "what has this agent been told?" is the question a mail list is opened to answer.
func (m *model) toggleMailAgent() {
	if m.mailAgent != "" {
		m.mailAgent = ""
		m.flash = "mail: every agent"
		return
	}
	msg, ok := m.selMail()
	if !ok {
		return
	}
	m.mailAgent = msg.Agent
	m.cursor[m.tab] = 0
	m.flash = "mail: " + msg.Agent + " only"
}
