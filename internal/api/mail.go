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

// Mail is one message in an agent's mailbox. Body is the FULL text where a detail view fetched it and
// a PREVIEW in a listing, which Truncated says: a rejection can run to hundreds of lines.
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
	// Pushed: also injected into the agent's session, so it may have been acted on live. "Pushed and
	// possibly missed" and "sitting here unread" are different diagnoses of the same quiet agent.
	Pushed bool `json:"pushed,omitempty"`
	// History is what happened to it, single fetches only. Pushed says typed, submitted and NOT
	// CONTRADICTED by the pane — a narrow pane or a dialog proves nothing either way (-> agent.inject).
	History []Event `json:"history,omitempty"`
}

// MailIDPrefix marks a mail id as an id, as sd-, td-, pr-, os- and gh- do: "reply to 47" is not an
// instruction beside agent names and ages, and "reply to ml-47" is.
const MailIDPrefix = "ml-"

// MailID renders a mail id for display and for anything the hub writes into a message.
func MailID(id int64) string { return fmt.Sprintf("%s%d", MailIDPrefix, id) }

// ParseMailID reads either spelling: the prefix is about how an id READS, and the bare form is already
// in shell history and in what agents have been told. It takes the WHOLE remainder or none of it —
// stopping at the first non-digit would read "ml-47zzz" as 47, and refusing says which part was wrong.
func ParseMailID(s string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(s), MailIDPrefix), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%q is not a mail id — they read like %s47", s, MailIDPrefix)
	}
	return id, nil
}

// MailToUser reports a message addressed to the USER rather than an agent — the only part of the
// mailbox a person is expected to read, and the rule behind the marker and both front-ends' split.
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
	// MailActive is unread OR read recently, as the Tasks segment of the same name is. It matters most
	// here because no mail is ever deleted: "all" grows for the life of the machine.
	MailActive MailFilter = "active"
	MailUnread MailFilter = "unread" // still waiting to be read
	MailAll    MailFilter = "all"    // every message, read or not
)

// MailFilters is the order both front-ends present, so the CLI's help and the TUI's cycle describe the
// same set the same way round. Active leads because it is what a view should OPEN on.
var MailFilters = []MailFilter{MailActive, MailUnread, MailAll}

// MatchesMailFilter reports whether m belongs in the view f names, narrowed to one recipient when agent
// is given ("" = all). An unrecognised filter admits everything: an empty listing would read as a lie.
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

// MailChangedAt is when a message last changed: READ if it has been, else sent. One sent days ago and
// read ten minutes ago changed ten minutes ago, which is what "recently" must mean to say anything.
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
