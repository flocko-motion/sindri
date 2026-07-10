// package: tui / tab_chat
// type:    ui (the Chat tab body)
// job:     render the user's chatroom — a members header plus the live transcript
//          (latest at the bottom), streamed in via BoardState.Chat. Composing is
//          enter -> a one-line input posted as the user (-> component_input); who's
//          in the room is curated from the CLI (`sindri chat add/remove`).
// limits:  view only; no membership editing, no scrollback (shows the tail).
package tui

import (
	"fmt"
	"strings"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

// chatBody renders the Chat tab: a fixed members header, a divider, then as much
// of the transcript tail as fits (newest at the bottom, like a chat log). The
// footer's "enter compose" hint drives posting.
func (m model) chatBody() string {
	v := m.state.Chat
	h := m.bodyHeight()
	header := []string{chatMembersLine(v), strings.Repeat("─", max(1, m.w))}
	avail := max(1, h-len(header))

	var msgs []string
	if len(v.Log) == 0 {
		msgs = []string{dimStyle.Render("(no messages yet — press enter to say something)")}
	} else {
		for _, msg := range v.Log {
			msgs = append(msgs, chatLine(msg))
		}
	}
	if len(msgs) > avail { // keep the newest that fit
		msgs = msgs[len(msgs)-avail:]
	}

	lines := append(header, msgs...)
	for len(lines) < h { // pad so the footer sits at the bottom
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = padTrunc(lines[i], m.w)
	}
	return strings.Join(lines, "\n")
}

// chatMembersLine summarizes who's in the room (name + role), or nudges the user
// to add someone when it's empty.
func chatMembersLine(v hub.ChatView) string {
	if len(v.Members) == 0 {
		return dimStyle.Render("Chatroom is empty — press enter and type /add <agent> to invite someone (/help for commands)")
	}
	names := make([]string, len(v.Members))
	for i, mem := range v.Members {
		if mem.Role != "" {
			names[i] = fmt.Sprintf("%s (%s)", mem.Name, mem.Role)
		} else {
			names[i] = mem.Name
		}
	}
	return fmt.Sprintf("Members (%d): %s", len(v.Members), strings.Join(names, ", "))
}

// chatLine formats one transcript message as "sender: body", collapsing any
// newlines in the body so a message stays a single row (width is clamped by the
// caller's padTrunc).
func chatLine(msg store.ChatMessage) string {
	return fmt.Sprintf("%s: %s", msg.Sender, strings.ReplaceAll(msg.Body, "\n", " "))
}
