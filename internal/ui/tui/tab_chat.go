// package: tui / tab_chat
// type:    ui (the Chat tab body)
// job:     render the user's chatroom — a members header plus the live transcript
//          (latest at the bottom), streamed in via BoardState.Chat. Composing is
//          enter -> a one-line input posted as the user (-> component_input); who's
//          in the room is curated from the CLI (`sindri chat add/remove`).
// limits:  view only; no membership editing, no scrollback (shows the tail).
package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/ui/theme"
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
		prev := ""
		for _, msg := range v.Log {
			msgs = append(msgs, chatLines(msg, prev)...)
			prev = msg.Sender
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
	// The user first: the stored roster holds only AGENTS, so an empty one means "no agents
	// yet", not an empty room — the user is always a participant. Same reasoning (and the
	// same shape) as the CLI's renderMembers.
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
		// The same interface description the CLI's join banner and /help print, so the room
		// reads identically wherever you meet it (-> hub.ChatHelpText).
		line += dimStyle.Render(" — no agents yet; press enter, then " + theme.HelpText)
	}
	return line
}

// chatLines formats one transcript message as a speaker header (time · icon · name, the
// name in its own deterministic colour from ui/theme — the same colour the CLI gives it)
// followed by the indented body, split on the body's own newlines so a multi-line message
// keeps its structure (the caller then word-wraps each line to the pane width).
//
// prev is the previous message's sender ("" for the first): a change of speaker gets a
// blank line above the header, and a run from one speaker repeats neither icon nor name.
// Grouping is what makes a long transcript skimmable instead of a wall of "name: text".
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
	// The words carry the speaker's colour too, not just the name — attribution has to
	// survive a multi-line message and the pane's word-wrap (ansi.Wrap preserves styles).
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
