// package: tui / component_pane
// type:    ui component (generic)
// job:     render text lines into a fixed width×height scrollable block via a
//          scroll.Viewport — the shared primitive behind the selector and the
//          detail pane. Each line is padded/truncated to width and the block
//          padded to the viewport height, so panes always fill their box.
// limits:  renders the given lines only; scroll state belongs to the
//          scroll.Viewport (-> scroll) and the content to the caller.
package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/flo-at/sindri/internal/ui/tui/scroll"
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

// wrapContent word-wraps each line to width so a pane shows the full text instead
// of truncating it with an ellipsis. ANSI-aware (a coloured diff keeps its colour
// across the wrap) and hard-breaks any token longer than width, so no line ever
// overflows. Returns a flat list of the wrapped lines, ready for pane().
func wrapContent(lines []string, width int) []string {
	wrapped, _ := wrapContentMapped(lines, width)
	return wrapped
}

// wrapContentMapped is wrapContent that also reports where each source line landed:
// origAt[i] is the index in the returned slice at which source line i begins. A
// caller tracking a per-line cursor or highlight (the detail pane's focused
// cross-reference) uses it to follow that line through the wrap.
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
	// A cell is ONE line, so fold any embedded newline. Text reaching a cell is often not
	// ours — an agent's activity log carries chat messages verbatim, and one of those ran to
	// 123 newlines. Measured as a single line but rendered as 124, it pushed the frame past
	// the terminal, which scrolled the top bar out of view for as long as that entry was in
	// the window. Folding here covers every cell rather than each caller remembering to.
	//
	// Before the ANSI check below, deliberately: a styled string (a dim timestamp, say) took
	// the early return and skipped this, which is exactly how the log entry got through.
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

// sgrMark stands in for a colour sequence while control bytes are stripped. A private-use
// rune, so it cannot collide with pane text.
const sgrMark = ""

// KeepColour reduces a string to text plus colour, dropping every escape that would command
// the terminal. Content captured from another program's screen — an agent's tmux pane — is
// its own full-screen drawing: cursor moves, erase-line and erase-display, scroll regions.
// Rendered into our frame those execute against OUR screen, wiping or shifting rows that
// belong to the layout, which is what makes the header appear to flicker or vanish while
// nothing about the board has changed. Colour is safe and worth keeping, so SGR stays.
//
// Apply it where foreign text ENTERS the model, so the renderer only ever sees content that
// obeys its cells.
func KeepColour(s string) string {
	s = ansiOSC.ReplaceAllString(s, "")
	s = ansiCSINonSGR.ReplaceAllString(s, "")
	s = ansiOther.ReplaceAllString(s, "")
	// Park the SGR sequences before the control-byte sweep, which would otherwise take the
	// ESC that begins them along with the bare BEL/CR/backspace bytes it is there to remove
	// (those move the cursor just as surely as a CSI does).
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
