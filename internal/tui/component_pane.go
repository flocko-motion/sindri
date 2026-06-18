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
	"github.com/charmbracelet/x/ansi"
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
		if i == cursor {
			// Strip any per-cell colour first so the highlight bar is a clean,
			// uninterrupted block (nested styles would otherwise reset its bg).
			out = append(out, selStyle.Render(padTrunc(ansi.Strip(lines[i]), width)))
			continue
		}
		out = append(out, padTrunc(lines[i], width))
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

// padTrunc fits s to exactly w display cells: too long is truncated with an
// ellipsis, too short is right-padded with spaces. ANSI-aware (lipgloss-styled
// lines keep their colour and count by display width), with tabs expanded and
// stray control chars dropped from plain lines first.
func padTrunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = sanitize(s)
	if width := ansi.StringWidth(s); width > w {
		return ansi.Truncate(s, w, "…")
	} else {
		return s + strings.Repeat(" ", w-width)
	}
}

// sanitize makes a line safe to render in a fixed-width cell. A line carrying an
// ANSI escape is lipgloss-styled and already well-formed — only its tabs are
// expanded. A plain line additionally has stray control chars (CR, …) dropped,
// since a raw tab overflows/wraps and a carriage return bleeds over the left
// edge (both common in diffs).
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\t", "    ")
	if strings.IndexByte(s, 0x1b) >= 0 { // contains ANSI — leave the sequences intact
		return s
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}
