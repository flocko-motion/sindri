// package: tui / tab_chat
// type:    ui (the Chat tab body)
// job:     render the user's chatroom — a members header plus the live transcript
// (latest at the bottom), streamed in via BoardState.Chat. Composing is
// enter -> a one-line input posted as the user (-> component_input); who's
// in the room is curated from the CLI (`sindri chat add/remove`).
// limits:  view only; no membership editing, no scrollback (shows the tail).
package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/chat"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// startComposing opens the multiline composer in the Chat tab's main pane and focuses it.
func (m *model) startComposing() tea.Cmd {
	m.sizeComposer()
	m.composer.Reset()
	m.composing = true
	return m.composer.Focus()
}

// sizeComposer fits the composer to the width and a slice of the body, leaving the transcript room.
func (m *model) sizeComposer() {
	m.composer.SetWidth(m.w)
	h := clampInt(m.bodyHeight()/3, 3, 8)
	m.composer.SetHeight(h)
}

// sendComposed closes the composer and posts in the background, so the keyboard comes back on the
// keystroke rather than after the hub has typed the line into every agent's session.
//
// The draft is kept, not cleared: a rejected message (over the length cap) is restored so the text
// can be trimmed instead of retyped. chatSentMsg clears it once the hub has taken it.
func (m model) sendComposed() (tea.Model, tea.Cmd) {
	v := strings.TrimSpace(m.composer.Value())
	m.composing = false
	m.composer.Blur()
	if v == "" {
		return m, nil
	}
	m.flash = "sending…" // acknowledges the keystroke, which is what the reporter could not see
	cl := m.cl
	if cl == nil {
		return m, nil
	}
	return m, func() tea.Msg {
		if err := cl.ChatSay(v); err != nil {
			return chatFailedMsg{err: err, draft: v}
		}
		return chatSentMsg{}
	}
}

// openNewMeetingChoice confirms the reset before it happens: clearing the shared history cannot be
// undone, and N sits next to the keys that send messages. The CLI's `meeting new` has no prompt —
// scripts shouldn't block — so this modal is where a human gets the guard.
func (m *model) openNewMeetingChoice() {
	cl := m.cl
	m.choice = choiceModalState{
		active:  true,
		title:   "start a new meeting? (clears the shared history for everyone; members stay)",
		options: []string{"cancel", "new meeting — clear the history"},
		values:  []string{"cancel", "new"},
		apply: func(v string) tea.Cmd {
			if v != "new" {
				return nil
			}
			return mutateThenRefresh(cl, cl.NewMeeting)
		},
	}
}

// updateComposer routes a keypress while composing: esc cancels, ctrl+s sends, ctrl+c quits. Enter
// is a newline for a message but submits a command — a one-line "/add nori" should not need ctrl+s.
func (m model) updateComposer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.composing = false
		m.composer.Blur()
		return m, nil
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "enter":
		if !chat.IsCommand(m.composer.Value()) {
			break // a message is multiline; only a command submits on enter
		}
		return m.sendComposed()
	case "ctrl+s":
		return m.sendComposed()
	}
	var cmd tea.Cmd
	m.composer, cmd = m.composer.Update(msg)
	return m, cmd
}

// chatBody renders the Chat tab: members header, divider, then the transcript tail, newest last.
func (m model) chatBody() string {
	v := m.state.Chat
	h := m.bodyHeight()

	// When composing, the editor takes the bottom of the pane; the transcript takes what's left.
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
		prev := ""
		for _, msg := range v.Log {
			msgs = append(msgs, chatLines(msg, prev)...)
			prev = msg.Sender
		}
		// Word-wrap to the pane width: a long message must read in full, not run off the edge.
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
	// The textarea manages its own width/cursor, so its lines go in raw — not through padTrunc.
	return strings.Join(append(tlines, composerLines...), "\n")
}

// chatMembersLine summarizes who's in the room, or nudges the user to add someone when empty.
func chatMembersLine(v hub.ChatView) string {
	// The user first: the roster holds only AGENTS, so an empty one means "no agents yet", not an
	// empty room — the user is always a participant. Same shape as the CLI's renderMembers.
	parts := []string{theme.Icon(theme.SenderUser) + " " +
		theme.NameStyle(theme.SenderUser).Render(theme.SenderUser) + dimStyle.Render(" (you)")}
	for _, mem := range v.Members {
		entry := theme.Icon(mem.Name) + " " + theme.NameStyle(mem.Name).Render(mem.Name)
		if mem.Role != "" {
			entry += dimStyle.Render(" (" + mem.Role + ")")
		}
		parts = append(parts, entry)
	}
	line := strings.Join(parts, " · ")
	if len(v.Members) == 0 {
		// The same description the CLI's join banner and /help print (-> hub.ChatHelpText).
		line += dimStyle.Render(" — no agents yet; press enter, then " + theme.HelpText)
	}
	return line
}

// chatLines formats one message as a speaker header (time · icon · name, the name in its own
// deterministic ui/theme colour, as in the CLI) plus the body, split on its own newlines. prev
// groups a speaker's run — no repeated header, a blank line on a change — so a transcript skims.
func chatLines(msg store.ChatMessage, prev string) []string {
	body := strings.Split(strings.TrimRight(msg.Body, "\n"), "\n")
	out := make([]string, 0, len(body)+2)
	if msg.Sender != prev {
		if prev != "" {
			out = append(out, "")
		}
		head := theme.Icon(msg.Sender) + " " + theme.NameStyle(msg.Sender).Render(msg.Sender)
		if hm := chatTime(msg.TS); hm != "" {
			head = dimStyle.Render(hm) + " " + head
		}
		out = append(out, head)
	}
	// The body carries the speaker's colour too, so attribution survives wrap (ansi.Wrap keeps it).
	style := theme.BodyStyle(msg.Sender)
	for _, seg := range body {
		if seg == "" {
			out = append(out, "") // a blank line needs no colour
			continue
		}
		out = append(out, "  "+style.Render(seg))
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
