// package: api / mail
// type:    logic (the mailbox wire type + its filters)
// job:     one message an agent must read as a view sees it, and the two axes a view
// narrows by — unread-or-all, and one agent — written once so a CLI flag and a
// TUI key filter the same set the same way.
// limits:  data and pure predicates; the mailbox itself is the hub's store.
package api

import (
	"fmt"
	"strings"
)

// Mail is one message in an agent's mailbox. Body is the FULL text where a detail view fetched it
// and a PREVIEW in a fleet listing, which is what Truncated says — a rejection carries its whole
// findings and can run to hundreds of lines, so a list that inlined every body would be unreadable
// before it was slow.
type Mail struct {
	ID      int64  `json:"id"`
	Project string `json:"project"`
	Repo    string `json:"repo"`   // the project's directory name, resolved by the hub (as on AgentView)
	Agent   string `json:"agent"`  // the recipient
	Sender  string `json:"sender"` // hub | user | reviewer | an agent's name
	Body    string `json:"body"`
	// Truncated: Body is a preview, and the whole message is a fetch away (-> client.MailBody).
	Truncated bool   `json:"truncated,omitempty"`
	SentAt    string `json:"sentAt"`
	ReadAt    string `json:"readAt,omitempty"` // "" = still unread
	// Pushed: the same message was also injected into the agent's session, so it may have been acted
	// on live. Carried because "pushed, and possibly missed" and "sitting here unread" are different
	// diagnoses, and a reader looking at a quiet agent needs to tell them apart.
	Pushed bool `json:"pushed,omitempty"`
}

// Read reports whether this message has been read.
func (m Mail) Read() bool { return m.ReadAt != "" }

// MailFilter names a segment of the mailbox.
type MailFilter string

// The segments. Their spelling is the CLI's flag value and the word the TUI footer shows.
const (
	MailUnread MailFilter = "unread" // still waiting to be read
	MailAll    MailFilter = "all"    // every message, read or not
)

// MailFilters is the order both front-ends present: the CLI lists it in its help, the TUI cycles
// through it, so the two describe the same set the same way round. Unread leads because it is the
// question worth asking of a mailbox.
var MailFilters = []MailFilter{MailUnread, MailAll}

// MatchesMailFilter reports whether m belongs in the view f names, narrowed to one recipient when
// agent is given ("" = every agent). An unrecognised filter admits everything: a listing that showed
// nothing would read as an empty mailbox, which is a lie a mistyped flag should not be able to tell.
func MatchesMailFilter(f MailFilter, agent string, m Mail) bool {
	if agent != "" && m.Agent != agent {
		return false
	}
	if f == MailUnread {
		return !m.Read()
	}
	return true
}

// FilterMail keeps the messages the filter admits, in the order given.
func FilterMail(f MailFilter, agent string, mail []Mail) []Mail {
	out := make([]Mail, 0, len(mail))
	for _, m := range mail {
		if MatchesMailFilter(f, agent, m) {
			out = append(out, m)
		}
	}
	return out
}

// NextMailFilter is the one after f, wrapping — the cycle a front-end walks. An unknown filter
// lands on the first, so a view can never get stuck outside the set.
func NextMailFilter(f MailFilter) MailFilter {
	for i, c := range MailFilters {
		if c == f {
			return MailFilters[(i+1)%len(MailFilters)]
		}
	}
	return MailFilters[0]
}

// ParseMailFilter reads a filter by name, naming the whole set when it is not one of them.
func ParseMailFilter(s string) (MailFilter, error) {
	for _, c := range MailFilters {
		if s == string(c) {
			return c, nil
		}
	}
	return "", fmt.Errorf("unknown mail filter %q — one of: %s", s, MailFilterNames())
}

// MailFilterNames lists the filters for help text and errors, in the shared order.
func MailFilterNames() string {
	names := make([]string, len(MailFilters))
	for i, c := range MailFilters {
		names[i] = string(c)
	}
	return strings.Join(names, "|")
}

// CountUnreadMail is how many of these messages are still unread.
func CountUnreadMail(mail []Mail) (n int) {
	for _, m := range mail {
		if !m.Read() {
			n++
		}
	}
	return n
}
