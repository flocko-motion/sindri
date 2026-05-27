package tui

import "github.com/charmbracelet/lipgloss"

// innerHeight returns the number of content lines available inside a bordered column.
// RoundedBorder = 2 lines (top + bottom), Padding(0,1) = 0 vertical padding.
func innerHeight(totalHeight int) int {
	h := totalHeight - 2
	if h < 1 {
		h = 1
	}
	return h
}

// clipLines truncates a slice of lines to at most max entries.
func clipLines(lines []string, max int) []string {
	if len(lines) <= max {
		return lines
	}
	return lines[:max]
}

var (
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	special   = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	dimmed    = lipgloss.AdaptiveColor{Light: "#A49FA5", Dark: "#777777"}

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(highlight).
			PaddingLeft(1)

	columnStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(0, 1)

	activeColumnStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(highlight).
				Padding(0, 1)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(highlight).
				Bold(true)

	normalItemStyle = lipgloss.NewStyle()

	dimStyle = lipgloss.NewStyle().
			Foreground(dimmed)

	statusRunning = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#73F59F")).
			Bold(true)

	statusOpen = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFA500"))

	statusDone = lipgloss.NewStyle().
			Foreground(dimmed)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(special).
			PaddingLeft(1).
			PaddingBottom(1)
)
