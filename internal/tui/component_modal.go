// package: tui / component_modal
// type:    ui component (generic, reusable)
// job:     an almost-full-screen modal — a bordered frame centered on the screen
//          with a title and scrollable content (via a scroll.Viewport). Used for
//          the detail overlay; reusable for any "expand to full screen" view.
// limits:  just the frame and scroll; the content shown is the caller's
//          (-> detailLines / the tabs).
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/tui/scroll"
)

// updateModal handles keys while the detail modal is open: scroll or close.
func (m model) updateModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "q":
		m.modal = false
		m.modalOverride, m.modalOverrideTitle = nil, ""
		m.reclamp() // restore the inline detail viewport
	case "j", "down":
		m.detail.ScrollDown()
	case "k", "up":
		m.detail.ScrollUp()
	case "ctrl+d":
		m.detail.ScrollPageDown()
	case "ctrl+u":
		m.detail.ScrollPageUp()
	case "g":
		m.detail.ScrollTop()
	case "G":
		m.detail.ScrollBottom()
	}
	return m, nil
}

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

// errModal renders a compact centered error box. The single presenter for any
// "that didn't work" feedback; dismissed with any key.
func errModal(msg string, screenW, screenH int) string {
	w := clampInt(screenW-10, 24, 72)
	body := lipgloss.NewStyle().Width(w).Render(msg)
	box := modalBorderStyle.BorderForeground(lipgloss.Color("203")).Render(
		errStyle.Render("⚠ error") + "\n\n" + body + "\n\n" + dimStyle.Render("any key to dismiss"))
	return lipgloss.Place(screenW, screenH, lipgloss.Center, lipgloss.Center, box)
}

// warnModal renders a compact centered warning box (orange), dismissed with any
// key. Used for non-fatal startup notices — e.g. an optional tool the project
// expects is missing — so the degrade is seen, not silent.
func warnModal(msg string, screenW, screenH int) string {
	w := clampInt(screenW-10, 24, 72)
	body := lipgloss.NewStyle().Width(w).Render(msg)
	box := modalBorderStyle.BorderForeground(cOrange).Render(
		lipgloss.NewStyle().Bold(true).Foreground(cOrange).Render("⚠ warning") + "\n\n" + body + "\n\n" + dimStyle.Render("any key to dismiss"))
	return lipgloss.Place(screenW, screenH, lipgloss.Center, lipgloss.Center, box)
}

// modalLines is the full-screen modal's content (the override, else the current
// tab's detail), word-wrapped to the modal's inner width so long lines (diffs,
// feedback) read in full instead of truncating. Used both to render the modal and
// to size its scroll viewport, so they always agree.
func (m model) modalLines() []string {
	lines := m.detailLines()
	if m.modalOverride != nil {
		lines = m.modalOverride
	}
	return wrapContent(lines, modalInnerWidth(m.w))
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
