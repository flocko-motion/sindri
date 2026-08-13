// package: tui / component_tabs
// type:    ui component (generic)
// job:     render the top header — the tab labels, the fleet's memory headroom, and a
// current-repo indicator. When a repo is active the WHOLE bar is background-filled
// with that repo's colour (a loud, always-visible "which repo am I in"); with no
// repo it falls back to the plain tab strip.
// limits:  pure rendering; which tab is active is the model's (-> tui.go), and the memory
// figure is the hub's, read off the board (-> api.BoardState.Memory).
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/theme"
)

var (
	activeTabStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("63"))
	tabStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// headerBar renders the top bar. With an active repo, the entire width is filled with
// the repo's primary colour and the label sits on it (the active tab underlined+bold
// to stand out on the shared background) — loud enough to never mistake which repo is
// in view. With no repo, it degrades to the classic tab strip.
func headerBar(labels []string, active, width int, repoName, repoTag string, repoColor int, mem api.FleetMemory) string {
	if repoName == "" {
		return plainTabStrip(labels, active, width, mem)
	}
	dark, bright := theme.RepoColors(repoTag, repoColor)
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
	badge := headroomBadge(mem, width-plainW-lipgloss.Width(ind))
	// Exactly `width` or the frame shears — one cell of overflow pushes the layout off-screen. So the
	// TAB STRIP gives way when the tabs stop fitting, keeping the repo indicator this bar exists to
	// shout; every tab added makes it tighter, which is why it degrades rather than overflows.
	strip, gap := b.String(), 0
	room := width - lipgloss.Width(ind) - lipgloss.Width(badge)
	if room < 0 {
		room = 0
	}
	if plainW > room {
		strip = ansi.Truncate(strip, room, "…")
	} else {
		gap = room - plainW
	}
	return strip + base.Render(strings.Repeat(" ", gap)) + base.Render(badge) + base.Bold(true).Render(ind)
}

// plainTabStrip is the no-repo fallback: labels with the active one highlighted,
// the headroom badge right-aligned, padded to width.
func plainTabStrip(labels []string, active, width int, mem api.FleetMemory) string {
	parts := make([]string, len(labels))
	for i, l := range labels {
		if i == active {
			parts[i] = activeTabStyle.Render(" " + l + " ")
		} else {
			parts[i] = tabStyle.Render(" " + l + " ")
		}
	}
	strip := strings.Join(parts, " ")
	badge := headroomBadge(mem, width-lipgloss.Width(strip))
	if pad := width - lipgloss.Width(strip) - lipgloss.Width(badge); pad > 0 {
		strip += strings.Repeat(" ", pad)
	}
	return strip + tabStyle.Render(badge)
}

// headroomBadge is the fleet's memory in the space left over after the tabs and the repo
// indicator, with a cell either side of it so it never abuts them. It yields that space rather
// than taking it: a header that pushed a tab off the edge to say how much memory is free would
// have cost more than it told anyone.
func headroomBadge(mem api.FleetMemory, space int) string {
	badge := theme.FleetBadge(mem, space-2)
	if badge == "" {
		return ""
	}
	return " " + badge + " "
}
