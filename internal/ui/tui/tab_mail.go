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
)

// mailVisible admits a message to the list: in scope, and admitted by the two filters — unread-or-all,
// and the recipient the user has narrowed to.
func (m model) mailVisible(msg api.Mail) bool {
	return m.inScope(msg.Project) && api.MatchesMailFilter(m.mailFilter, m.mailAgent, msg)
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

func (m model) mailRows() []row {
	var out []row
	for _, msg := range m.mailShown() {
		state, st := "unread", stWarn
		if msg.Read() {
			state, st = "read", stDone
		}
		if msg.Pushed { // also injected live, so it may already have been acted on
			state += "+push"
		}
		out = append(out, row{strings.Join([]string{
			m.repoStyle(msg.Project).Render(fmt.Sprintf("%-10.10s", msg.Repo)),
			fmt.Sprintf("%-12s", msg.Agent),
			fmt.Sprintf("%-10s", dash(msg.Sender)),
			st.Render(fmt.Sprintf("%-12s", state)),
			dimStyle.Render(fmt.Sprintf("%5s", shortAge(msg.SentAt))),
			oneLineText(msg.Body),
		}, " "), fmt.Sprint(msg.ID)})
	}
	// The window is not the history: a list that stopped at its rows would present the recent end as
	// everything, and finding last month's message is the whole reason nothing is deleted.
	if n, total := len(m.state.Mail), m.state.MailTotal; total > n {
		out = append(out, row{dimStyle.Render(fmt.Sprintf("… showing the last %d of %d messages — older mail: `sindri mail show <id>`", n, total)), ""})
	}
	return out
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
