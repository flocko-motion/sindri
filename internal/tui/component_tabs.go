// package: tui / component_tabs
// type:    ui component (generic)
// job:     render the top tab strip — each label as `[<n> Title]`, the active
//          one highlighted — padded to the full width.
// limits:  pure rendering; which tab is active is the model's (-> tui.go).
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	activeTabStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("63"))
	tabStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// tabStrip renders labels with the active index highlighted, padded to width.
func tabStrip(labels []string, active, width int) string {
	parts := make([]string, len(labels))
	for i, l := range labels {
		if i == active {
			parts[i] = activeTabStyle.Render(" " + l + " ")
		} else {
			parts[i] = tabStyle.Render(" " + l + " ")
		}
	}
	strip := strings.Join(parts, " ")
	// Pad by display width (lipgloss.Width ignores the ANSI styling).
	if pad := width - lipgloss.Width(strip); pad > 0 {
		strip += strings.Repeat(" ", pad)
	}
	return strip
}
