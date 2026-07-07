// package: tui / component_choice
// type:    ui component (generic, reusable)
// job:     a centered multiple-choice modal — a titled bordered box listing
//          options with a cursor. Reusable for any pick-one prompt (priority,
//          status, role, repo switcher). Long lists scroll; a `filterable` prompt
//          (the repo switcher) narrows by typeahead.
// limits:  generic chrome + keys; what the options mean and what a pick does
//          belong to the caller (-> the tab that opens it).
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// choiceModalState is a generic pick-one prompt: options, parallel values, and
// what to do with the chosen value. When filterable, typing narrows the list by a
// case-insensitive substring match (for a long list like the repo switcher).
type choiceModalState struct {
	active     bool
	title      string
	options    []string
	values     []string
	cursor     int
	filterable bool
	filter     string
	apply      func(value string) tea.Cmd
}

// visible returns the options/values that match the current filter (all of them
// when not filtering), preserving order.
func (c choiceModalState) visible() (opts, vals []string) {
	if c.filter == "" {
		return c.options, c.values
	}
	needle := strings.ToLower(c.filter)
	for i, o := range c.options {
		if strings.Contains(strings.ToLower(o), needle) {
			opts = append(opts, o)
			vals = append(vals, c.values[i])
		}
	}
	return opts, vals
}

// updateChoice handles keys while a pick-one modal is open.
func (m model) updateChoice(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	opts, vals := m.choice.visible()
	switch msg.String() {
	case "esc":
		m.choice.active = false
	case "ctrl+c":
		m.choice.active = false
	case "down", "ctrl+n":
		if m.choice.cursor < len(opts)-1 {
			m.choice.cursor++
		}
	case "up", "ctrl+p":
		if m.choice.cursor > 0 {
			m.choice.cursor--
		}
	case "enter":
		if len(vals) == 0 {
			return m, nil // nothing matches the filter — no-op
		}
		apply := m.choice.apply
		val := vals[clampInt(m.choice.cursor, 0, len(vals)-1)]
		m.choice.active = false
		return m, apply(val)
	case "backspace":
		if m.choice.filterable && m.choice.filter != "" {
			m.choice.filter = m.choice.filter[:len(m.choice.filter)-1]
			m.choice.cursor = 0
		}
	default:
		// A filterable prompt captures printable runs as typeahead; j/k stay as
		// navigation only for non-filterable prompts (where letters can't be text).
		if m.choice.filterable {
			if r := msg.Runes; len(r) == 1 && r[0] >= ' ' {
				m.choice.filter += string(r)
				m.choice.cursor = 0
			}
		} else {
			switch msg.String() {
			case "j":
				if m.choice.cursor < len(opts)-1 {
					m.choice.cursor++
				}
			case "k":
				if m.choice.cursor > 0 {
					m.choice.cursor--
				}
			}
		}
	}
	return m, nil
}

// choiceModal renders the pick-one modal: title, an optional filter line, the
// (filtered) options with the cursor row highlighted and a scroll window for long
// lists, and a key hint.
func choiceModal(c choiceModalState, screenW, screenH int) string {
	opts, _ := c.visible()
	cursor := clampInt(c.cursor, 0, max(len(opts)-1, 0))

	hint := "j/k·↑/↓ move · enter select · esc cancel"
	if c.filterable {
		hint = "type to filter · ↑/↓ move · enter select · esc cancel"
	}
	title := c.title
	if c.filterable {
		title += "  /" + c.filter
	}

	cw := max(lipgloss.Width(title), lipgloss.Width(hint))
	for _, o := range opts {
		if w := lipgloss.Width(o) + 2; w > cw {
			cw = w
		}
	}
	blank := strings.Repeat(" ", cw)

	// Scroll window: bound the option rows to what fits, keeping the cursor visible.
	maxRows := screenH - 6 // title, two blanks, hint, border top/bottom
	if maxRows < 3 {
		maxRows = 3
	}
	start, end := 0, len(opts)
	if len(opts) > maxRows {
		start = clampInt(cursor-maxRows/2, 0, len(opts)-maxRows)
		end = start + maxRows
	}

	lines := []string{modalTitleStyle.Render(padTrunc(title, cw)), blank}
	if start > 0 {
		lines = append(lines, dimStyle.Render(padTrunc("  ↑ more", cw)))
	}
	for i := start; i < end; i++ {
		if i == cursor {
			lines = append(lines, selStyle.Render(padTrunc("▸ "+opts[i], cw)))
		} else {
			lines = append(lines, padTrunc("  "+opts[i], cw))
		}
	}
	if end < len(opts) {
		lines = append(lines, dimStyle.Render(padTrunc("  ↓ more", cw)))
	}
	if len(opts) == 0 {
		lines = append(lines, dimStyle.Render(padTrunc("  (no matches)", cw)))
	}
	lines = append(lines, blank, dimStyle.Render(padTrunc(hint, cw)))
	box := modalBorderStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(screenW, screenH, lipgloss.Center, lipgloss.Center, box)
}
