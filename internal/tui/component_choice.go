// package: tui / component_choice
// type:    ui component (generic, reusable)
// job:     a centered multiple-choice modal — a titled bordered box listing
//          options with a cursor. Reusable for any pick-one prompt (priority,
//          status, role, …).
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// updateChoice handles keys while a pick-one modal is open.
func (m model) updateChoice(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.choice.active = false
	case "j", "down":
		if m.choice.cursor < len(m.choice.options)-1 {
			m.choice.cursor++
		}
	case "k", "up":
		if m.choice.cursor > 0 {
			m.choice.cursor--
		}
	case "enter":
		val := m.choice.values[m.choice.cursor]
		apply := m.choice.apply
		m.choice.active = false
		return m, apply(val)
	}
	return m, nil
}

// choiceModal renders a centered pick-one modal: title, options (cursor row
// highlighted), and a key hint.
func choiceModal(title string, options []string, cursor, screenW, screenH int) string {
	hint := "j/k·↑/↓ · enter select · esc cancel"
	cw := max(lipgloss.Width(title), lipgloss.Width(hint))
	for _, o := range options {
		if c := lipgloss.Width(o) + 2; c > cw {
			cw = c
		}
	}
	// Pre-pad each (plain) line to cw, then style — so nothing wraps and the box
	// sizes to its content.
	blank := strings.Repeat(" ", cw)
	lines := []string{modalTitleStyle.Render(padTrunc(title, cw)), blank}
	for i, o := range options {
		if i == cursor {
			lines = append(lines, selStyle.Render(padTrunc("▸ "+o, cw)))
		} else {
			lines = append(lines, padTrunc("  "+o, cw))
		}
	}
	lines = append(lines, blank, dimStyle.Render(padTrunc(hint, cw)))
	box := modalBorderStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(screenW, screenH, lipgloss.Center, lipgloss.Center, box)
}
