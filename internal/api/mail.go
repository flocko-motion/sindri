// package: api / mail
// type:    logic (the mailbox wire type + its filters)
// job:     one message an agent must read as a view sees it, and the two axes a view
// narrows by — unread-or-all, and one agent — written once so a CLI flag and a
// TUI key filter the same set the same way.
// limits:  data and pure predicates; the mailbox itself is the hub's store.
package api

import (
	"fmt"
	"strconv"
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
	// InReplyTo is the message this one answers, 0 when it starts a thread — so a front-end can show an
	// exchange as one, and a recipient is not left matching a reply against its own messages by hand.
	InReplyTo int64 `json:"inReplyTo,omitempty"`
	// Pushed: the same message was also injected into the agent's session, so it may have been acted
	// on live. Carried because "pushed, and possibly missed" and "sitting here unread" are different
	// diagnoses, and a reader looking at a quiet agent needs to tell them apart.
	Pushed bool `json:"pushed,omitempty"`
}

// MailIDPrefix marks a mail id as an id. Every other id in sindri carries one — sd-, td-, pr-, os-,
// gh- — and a bare integer beside agent names, repo names and ages does not read as something you can
// address: "reply to 47" is not an instruction, "reply to ml-47" is.
const MailIDPrefix = "ml-"

// MailID renders a mail id for display and for anything the hub writes into a message.
func MailID(id int64) string { return fmt.Sprintf("%s%d", MailIDPrefix, id) }

// ParseMailID reads either spelling. The BARE form keeps working because it is already in users'
// shell history and in whatever agents have been told — breaking that to gain a prefix would be a poor
// trade, and the prefix is about how an id READS, not about what is accepted.
//
// It reads the WHOLE remainder or none of it: a scan that stops at the first non-digit takes "ml-47zzz"
// for 47, and half-reading an argument an agent types from memory is worse than refusing it, since the
// refusal is the only thing that says which part was wrong.
func ParseMailID(s string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(s), MailIDPrefix), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%q is not a mail id — they read like %s47", s, MailIDPrefix)
	}
	return id, nil
}

// MailToUser reports whether a message is addressed to the USER rather than to an agent — the only
// part of the mailbox a person is expected to read, and the rule behind both the marker and the way
// each front-end separates those rows out.
func MailToUser(m Mail) bool { return m.Agent == SenderUser }

// MailToUserHeading labels the messages addressed to the user, lifted above the rest of the
// mailbox — the few rows a person is expected to read, out of a list that is mostly agent traffic.
func MailToUserHeading(n int) string {
	return fmt.Sprintf("To you (%d):", n)
}

// MailLogHeading labels what's under MailToUserHeading: the record of what every agent has been
// told, shown only once there is a "to you" section above it to separate it from.
const MailLogHeading = "The rest of the mailbox:"

// Read reports whether this message has been read.
func (m Mail) Read() bool { return m.ReadAt != "" }

// MailFilter names a segment of the mailbox.
type MailFilter string

// The segments. Their spelling is the CLI's flag value and the word the TUI footer shows.
const (
	// MailActive is unread OR read recently — the mail equivalent of the Tasks segment of the same
	// name, where unread is "open" and read is "closed". It matters more here than anywhere else
	// because no mail is ever deleted: "all" grows for the life of the machine, so it is the one view
	// that gets less usable every day, and a bounded default is what keeps the tab readable a year on.
	MailActive MailFilter = "active"
	MailUnread MailFilter = "unread" // still waiting to be read
	MailAll    MailFilter = "all"    // every message, read or not
)

// MailFilters is the order both front-ends present: the CLI lists it in its help, the TUI cycles
// through it, so the two describe the same set the same way round. Active leads because it is what a
// view should OPEN on; unread stays, being still the sharpest question to ask of a mailbox.
var MailFilters = []MailFilter{MailActive, MailUnread, MailAll}

// MatchesMailFilter reports whether m belongs in the view f names, narrowed to one recipient when
// agent is given ("" = every agent). An unrecognised filter admits everything: a listing that showed
// nothing would read as an empty mailbox, which is a lie a mistyped flag should not be able to tell.
func MatchesMailFilter(f MailFilter, agent string, m Mail) bool {
	if agent != "" && m.Agent != agent {
		return false
	}
	switch f {
	case MailUnread:
		return !m.Read()
	case MailActive:
		// A UNION, as it is for tasks: everything still waiting, plus whatever changed inside the
		// window — so a message read a moment ago stays on screen instead of vanishing as it is read.
		return !m.Read() || changedWithin(MailChangedAt(m), ActiveWindow)
	}
	return true
}

// MailChangedAt is when a message last changed: when it was READ if it has been, else when it was
// sent. A message sent days ago and read ten minutes ago changed ten minutes ago, and that is what
// "recently" has to mean for the active segment to say anything useful.
func MailChangedAt(m Mail) string {
	if m.ReadAt != "" {
		return m.ReadAt
	}
	return m.SentAt
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
