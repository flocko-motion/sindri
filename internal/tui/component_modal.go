// package: tui / component_modal
// type:    ui component (generic, reusable)
// job:     an almost-full-screen modal — a bordered frame centered on the screen
//          with a title and scrollable content (via a scroll.Viewport). Used for
//          the detail overlay; reusable for any "expand to full screen" view.
package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/tui/scroll"
)

var (
	modalBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("63")).Padding(0, 1)
	modalTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
)

// modalContentHeight is how many content rows the modal shows on a screenH-tall
// screen (leaving room for the border, title, hint, and a one-row margin).
func modalContentHeight(screenH int) int {
	if h := screenH - 6; h > 3 {
		return h
	}
	return 3
}

// modal renders content as an almost-full-screen centered modal. vp scrolls the
// content (its Height should be modalContentHeight(screenH)).
func modal(title string, content []string, vp scroll.Viewport, screenW, screenH int) string {
	cw := screenW - 6 // 1-col margin + border + 1-col padding, each side
	if cw < 10 {
		cw = 10
	}
	lines := make([]string, len(content))
	for i, l := range content {
		lines[i] = padTrunc(l, cw)
	}
	inner := vp.Render(lines)
	hint := dimStyle.Render(padTrunc("j/k·↑/↓ scroll · g/G ends · esc/enter close", cw))
	box := modalBorderStyle.Width(cw).Render(
		modalTitleStyle.Render(padTrunc(title, cw)) + "\n" + inner + "\n" + hint)
	return lipgloss.Place(screenW, screenH, lipgloss.Center, lipgloss.Center, box)
}
