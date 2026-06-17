// package: tui / component_pane
// type:    ui component (generic)
// job:     render a slice of text lines into a fixed width×height scrollable
//          block via a scroll.Viewport — the shared primitive behind both the
//          selector and the detail pane. Optionally highlights the cursor row.
//          Every line is padded/truncated to width and the block padded to the
//          viewport height, so panes always fill their box.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/tui/scroll"
)

var (
	selStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("238"))
	dimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	divStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
)

// pane renders lines through vp into a width×vp.Height block. cursor is the
// highlighted line index (-1 for none).
func pane(lines []string, vp scroll.Viewport, width, cursor int) string {
	start, end := vp.Window()
	out := make([]string, 0, vp.Height)
	for i := start; i < end && i < len(lines); i++ {
		s := padTrunc(lines[i], width)
		if i == cursor {
			s = selStyle.Render(s)
		}
		out = append(out, s)
	}
	blank := strings.Repeat(" ", width)
	for len(out) < vp.Height {
		out = append(out, blank)
	}
	return strings.Join(out, "\n")
}

// divider is a vertical rule of h rows.
func divider(h int) string {
	rows := make([]string, h)
	for i := range rows {
		rows[i] = divStyle.Render("│")
	}
	return strings.Join(rows, "\n")
}

// padTrunc fits s to exactly w display cells (rune-count approximation): too long
// is truncated with an ellipsis, too short is right-padded with spaces.
func padTrunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > w {
		if w == 1 {
			return "…"
		}
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-len(r))
}
