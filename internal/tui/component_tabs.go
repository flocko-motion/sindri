// package: tui / component_tabs
// type:    ui component (generic)
// job:     render the top header — the tab labels plus a current-repo indicator. When
//          a repo is active the WHOLE bar is background-filled with that repo's colour
//          (a loud, always-visible "which repo am I in"); with no repo it falls back
//          to the plain tab strip.
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

// headerBar renders the top bar. With an active repo, the entire width is filled with
// the repo's primary colour and the label sits on it (the active tab underlined+bold
// to stand out on the shared background) — loud enough to never mistake which repo is
// in view. With no repo, it degrades to the classic tab strip.
func headerBar(labels []string, active, width int, repoName, repoTag string) string {
	if repoName == "" {
		return plainTabStrip(labels, active, width)
	}
	dark, bright := repoColors(repoTag)
	base := lipgloss.NewStyle().Background(dark).Foreground(bright)
	activeSeg := lipgloss.NewStyle().Background(bright).Foreground(dark).Bold(true) // inverted block

	var b strings.Builder
	plainW := 0
	for i, l := range labels {
		seg := " " + l + " "
		if i == active {
			b.WriteString(activeSeg.Render(seg))
		} else {
			b.WriteString(base.Render(seg))
		}
		b.WriteString(base.Render(" ")) // colored separator between tabs
		plainW += lipgloss.Width(seg) + 1
	}
	ind := "◉ " + repoName + " "
	gap := width - plainW - lipgloss.Width(ind)
	if gap < 1 {
		gap = 1
	}
	b.WriteString(base.Render(strings.Repeat(" ", gap)))
	b.WriteString(base.Bold(true).Render(ind))
	return b.String()
}

// plainTabStrip is the no-repo fallback: labels with the active one highlighted,
// padded to width.
func plainTabStrip(labels []string, active, width int) string {
	parts := make([]string, len(labels))
	for i, l := range labels {
		if i == active {
			parts[i] = activeTabStyle.Render(" " + l + " ")
		} else {
			parts[i] = tabStyle.Render(" " + l + " ")
		}
	}
	strip := strings.Join(parts, " ")
	if pad := width - lipgloss.Width(strip); pad > 0 {
		strip += strings.Repeat(" ", pad)
	}
	return strip
}
