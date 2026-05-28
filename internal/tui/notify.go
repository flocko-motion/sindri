// package: tui / notify
// type:    ui
// job:     the bottom notification/flash bar — transient status messages.
// limits:  presentation only; no domain logic.
package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const flashDuration = 2 * time.Second

var (
	notifyFlashStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#7D56F4")).
				PaddingLeft(1).
				PaddingRight(1)

	notifyDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#777777")).
			PaddingLeft(1)

	notifyErrorFlashStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#FF0000")).
				PaddingLeft(1).
				PaddingRight(1)

	notifyErrorDimStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF6666")).
				PaddingLeft(1)
)

type notification struct {
	message string
	isError bool
	time    time.Time
}

type notifyMsg struct {
	message string
	isError bool
}

type flashExpiredMsg struct{}

func flashTimer() tea.Cmd {
	return tea.Tick(flashDuration, func(time.Time) tea.Msg {
		return flashExpiredMsg{}
	})
}

func (n notification) isFlashing() bool {
	return time.Since(n.time) < flashDuration
}

func (n notification) render(width int) string {
	if n.message == "" {
		return lipgloss.NewStyle().Width(width).Render("")
	}
	if n.isFlashing() {
		if n.isError {
			return notifyErrorFlashStyle.Width(width).Render(n.message)
		}
		return notifyFlashStyle.Width(width).Render(n.message)
	}
	if n.isError {
		return notifyErrorDimStyle.Width(width).Render(n.message)
	}
	return notifyDimStyle.Width(width).Render(n.message)
}
