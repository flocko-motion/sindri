package tui

import "github.com/charmbracelet/lipgloss"

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
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#7D56F4")).
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

	statusOrphaned = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6666")).
			Bold(true)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(special).
			PaddingLeft(1).
			PaddingBottom(1)
)
