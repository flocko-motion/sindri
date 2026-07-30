// package: tui / component_pane
// type:    ui component (generic)
// job:     render lines into a fixed width×height scrollable block via a scroll.Viewport —
// the primitive behind the selector and the detail pane. Every line is fitted to
// width and the block padded to height, so a pane always fills its box exactly.
// limits:  renders the given lines only; scroll state is the Viewport's, content the caller's.
package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/flo-at/sindri/internal/ui/tui/scroll"
)

var (
	selStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("238"))
	dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	divStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
)

// pane renders lines through vp into a width×vp.Height block. cursor is the
// highlighted line index (-1 for none).
func pane(lines []string, vp scroll.Viewport, width, cursor int) string {
	start, end := vp.Window()
	out := make([]string, 0, vp.Height)
	for i := start; i < end && i < len(lines); i++ {
		if i == cursor {
			// Strip per-cell colour: a nested style would reset the highlight bar's background.
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

// wrapContent word-wraps each line to width, so a pane shows full text rather than an
// ellipsis. ANSI-aware, and hard-breaks an over-long token so no line overflows.
func wrapContent(lines []string, width int) []string {
	wrapped, _ := wrapContentMapped(lines, width)
	return wrapped
}

// wrapContentMapped is wrapContent that also reports where each source line landed, so a
// caller tracking a cursor or highlight can follow its line through the wrap.
func wrapContentMapped(lines []string, width int) (wrapped []string, origAt []int) {
	origAt = make([]int, len(lines))
	if width <= 0 {
		for i := range lines {
			origAt[i] = i
		}
		return lines, origAt
	}
	for i, l := range lines {
		origAt[i] = len(wrapped)
		wrapped = append(wrapped, strings.Split(ansi.Wrap(l, width, ""), "\n")...)
	}
	return wrapped, origAt
}

// divider is a vertical rule of h rows.
func divider(h int) string {
	rows := make([]string, h)
	for i := range rows {
		rows[i] = divStyle.Render("│")
	}
	return strings.Join(rows, "\n")
}

// padTrunc fits s to exactly w display cells — truncated with an ellipsis, or space-padded.
// ANSI-aware, and sanitized first so nothing in it can escape its cell.
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

// sanitize makes a line safe for a fixed-width cell: tabs expanded, and control chars dropped
// from a plain line (a CR bleeds past the left edge). A styled line keeps its escapes.
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\t", "    ")
	// A cell is ONE line: a newline in foreign text (an activity log quotes chat verbatim)
	// renders as a row the layout never counted. Folded before the ANSI early exit below.
	if strings.ContainsAny(s, "\n\r") {
		s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	}
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

// Escape sequences that act on the TERMINAL rather than on the text: cursor motion, erase,
// scroll regions, mode switches, OSC strings, charset selection. In CSI form the final byte
// identifies the function, and only 'm' (SGR) is presentation.
var (
	ansiCSINonSGR = regexp.MustCompile(`\x1b\[[0-9;:?!"'$ >=<]*[@-ln-~]`)
	ansiOSC       = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	ansiOther     = regexp.MustCompile(`\x1b(?:[()*+][0-9A-Za-z]|[=>78MDEHZc]|#[0-9])`)
	ansiSGR       = regexp.MustCompile(`\x1b\[[0-9;:]*m`)
)

// sgrMark stands in for a colour sequence while control bytes are stripped — a private-use
// rune, so it cannot collide with pane text.
const sgrMark = ""

// KeepColour reduces foreign text to text plus colour. A captured screen is another program's
// full-screen drawing, and its cursor moves and erases would execute against OUR frame. Apply
// it where such text enters the model.
func KeepColour(s string) string {
	s = ansiOSC.ReplaceAllString(s, "")
	s = ansiCSINonSGR.ReplaceAllString(s, "")
	s = ansiOther.ReplaceAllString(s, "")
	// Park the colour sequences: the sweep below drops the ESC that begins them along with the
	// bare control bytes it is there to remove.
	var kept []string
	s = ansiSGR.ReplaceAllStringFunc(s, func(seq string) string {
		kept = append(kept, seq)
		return sgrMark
	})
	s = strings.Map(func(r rune) rune {
		if r == 0x1b || (r < 0x20 && r != '\n') || r == 0x7f {
			return -1
		}
		return r
	}, s)
	for _, seq := range kept {
		s = strings.Replace(s, sgrMark, seq, 1)
	}
	return s
}
