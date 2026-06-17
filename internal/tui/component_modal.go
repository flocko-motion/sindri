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

// modalWidth is the bordered-box content width (the Style.Width) of an
// almost-full-screen modal on screenW.
func modalWidth(screenW int) int {
	if cw := screenW - 6; cw > 12 { // 1-col margin + border + 1-col padding, each side
		return cw
	}
	return 12
}

// modalInnerWidth is the usable text width inside the frame — modalWidth minus
// the box's horizontal padding (1 each side). Body/footer/title lines must be
// padded to this so full-width (e.g. highlighted) lines don't wrap.
func modalInnerWidth(screenW int) int { return modalWidth(screenW) - 2 }

// modalFrame is the shared almost-full-screen chrome: a centered bordered box
// with a title, a pre-sized body block, and a pre-styled footer line. Both the
// scrollable detail modal and the fill-in form compose this — neither draws its
// own border. body and footer must already be styled and padded to
// modalInnerWidth(screenW).
func modalFrame(title, body, footer string, screenW, screenH int) string {
	iw := modalInnerWidth(screenW)
	box := modalBorderStyle.Width(modalWidth(screenW)).Render(
		modalTitleStyle.Render(padTrunc(title, iw)) + "\n" + body + "\n" + footer)
	return lipgloss.Place(screenW, screenH, lipgloss.Center, lipgloss.Center, box)
}

// modal renders content as an almost-full-screen centered modal. vp scrolls the
// content (its Height should be modalContentHeight(screenH)).
func modal(title string, content []string, vp scroll.Viewport, screenW, screenH int) string {
	iw := modalInnerWidth(screenW)
	lines := make([]string, len(content))
	for i, l := range content {
		lines[i] = padTrunc(l, iw)
	}
	hint := dimStyle.Render(padTrunc("j/k·↑/↓ scroll · g/G ends · esc/enter close", iw))
	return modalFrame(title, vp.Render(lines), hint, screenW, screenH)
}
