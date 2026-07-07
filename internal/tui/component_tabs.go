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

// headerBar renders the tab strip with a persistent current-repo indicator pinned to
// the right, colored by the repo's own scheme — the always-visible "which repo am I
// in" (herdr/tmux-style). An empty repoName just pads the strip to width.
func headerBar(labels []string, active, width int, repoName, repoTag string) string {
	parts := make([]string, len(labels))
	for i, l := range labels {
		if i == active {
			parts[i] = activeTabStyle.Render(" " + l + " ")
		} else {
			parts[i] = tabStyle.Render(" " + l + " ")
		}
	}
	strip := strings.Join(parts, " ")
	if repoName != "" {
		ind := projectStyle(repoTag).Bold(true).Render("◉ " + repoName)
		if gap := width - lipgloss.Width(strip) - lipgloss.Width(ind); gap >= 1 {
			return strip + strings.Repeat(" ", gap) + ind
		}
		// Too narrow to fit both — the indicator wins (it's the load-bearing signal).
		return strip + " " + ind
	}
	// Pad by display width (lipgloss.Width ignores the ANSI styling).
	if pad := width - lipgloss.Width(strip); pad > 0 {
		strip += strings.Repeat(" ", pad)
	}
	return strip
}
