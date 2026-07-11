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
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

// startComposing opens the multiline composer in the Chat tab's main pane and
// focuses it. Returns the cursor-blink cmd.
func (m *model) startComposing() tea.Cmd {
	m.sizeComposer()
	m.composer.Reset()
	m.composing = true
	return m.composer.Focus()
}

// sizeComposer sizes the composer to the terminal width and a modest slice of the
// body height (so the transcript stays readable above it).
func (m *model) sizeComposer() {
	m.composer.SetWidth(m.w)
	h := clampInt(m.bodyHeight()/3, 3, 8)
	m.composer.SetHeight(h)
}

// updateComposer routes a keypress while the composer is open: esc cancels, ctrl+s
// sends (enter inserts a newline — this is multiline), ctrl+c still quits; anything
// else edits. Sending goes through the hub, which enforces the length cap and hands
// back "too long" feedback rather than truncating.
func (m model) updateComposer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.composing = false
		m.composer.Blur()
		return m, nil
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "ctrl+s":
		v := strings.TrimSpace(m.composer.Value())
		if v == "" || m.cl == nil {
			m.composing = false
			m.composer.Blur()
			return m, nil
		}
		cl := m.cl
		return m, func() tea.Msg {
			if err := cl.ChatSay(v); err != nil {
				return errModalMsg{err} // e.g. "too long" — shown; the draft stays open to trim
			}
			return chatSentMsg{}
		}
	}
	var cmd tea.Cmd
	m.composer, cmd = m.composer.Update(msg)
	return m, cmd
}

// chatBody renders the Chat tab: a fixed members header, a divider, then as much
// of the transcript tail as fits (newest at the bottom, like a chat log). The
// footer's "enter compose" hint drives posting.
func (m model) chatBody() string {
	v := m.state.Chat
	h := m.bodyHeight()

	// When composing, the multiline editor occupies the bottom of the pane (with a
	// divider above it); the transcript takes what's left.
	var composerLines []string
	transcriptH := h
	if m.composing {
		composerLines = append([]string{strings.Repeat("─", max(1, m.w))}, strings.Split(m.composer.View(), "\n")...)
		transcriptH = max(1, h-len(composerLines))
	}

	header := []string{chatMembersLine(v), strings.Repeat("─", max(1, m.w))}
	avail := max(1, transcriptH-len(header))
	var msgs []string
	if len(v.Log) == 0 {
		msgs = []string{dimStyle.Render("(no messages yet — press enter to say something)")}
	} else {
		for _, msg := range v.Log {
			msgs = append(msgs, chatLines(msg)...)
		}
		// Word-wrap to the pane width so long messages are readable in full instead
		// of running off the edge (the whole point of a chat you can follow).
		msgs = wrapContent(msgs, max(1, m.w))
	}
	if len(msgs) > avail { // keep the newest that fit
		msgs = msgs[len(msgs)-avail:]
	}

	tlines := append(header, msgs...)
	for len(tlines) < transcriptH { // pad the transcript region to its full height
		tlines = append(tlines, "")
	}
	tlines = tlines[:transcriptH]
	for i := range tlines {
		tlines[i] = padTrunc(tlines[i], m.w)
	}
	// The composer lines render themselves (textarea manages its own width/cursor),
	// so they're appended raw — not run through padTrunc.
	return strings.Join(append(tlines, composerLines...), "\n")
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

// chatLines formats one transcript message as "HH:MM sender: body", split on the
// body's own newlines so a multi-line message keeps its structure (the caller then
// word-wraps each line to the pane width). The timestamp shows WHEN it was said.
func chatLines(msg store.ChatMessage) []string {
	head := msg.Sender + ": "
	if hm := chatTime(msg.TS); hm != "" {
		head = hm + " " + head
	}
	body := strings.Split(msg.Body, "\n")
	out := make([]string, 0, len(body))
	for i, seg := range body {
		if i == 0 {
			out = append(out, head+seg)
		} else {
			out = append(out, strings.Repeat(" ", len(head))+seg) // indent continuation under the text
		}
	}
	return out
}

// chatTime renders a stored RFC3339 timestamp as local HH:MM ("" if unparseable).
func chatTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}
	return t.Local().Format("15:04")
}
