// package: tui/scroll / scroll
// type:    ui primitive (pure, no Bubble Tea coupling)
// job:     a fixed-height scrollable pane. Given a Height and Total line count,
//          it tracks the visible window (and an optional cursor that the window
//          follows), clamped against every edge, and renders content of any
//          length into exactly Height rows — padding when shorter, scrolling
//          when longer. The single home for viewport arithmetic so no pane
//          reimplements (and re-breaks) it.
// limits:  width/styling is the caller's job; this owns vertical geometry only.
package scroll

import "strings"

// Viewport is a fixed-height view over Total lines. Cursor-follow mode uses
// Cursor + Up/Down/etc.; free-scroll mode uses Scroll* and ignores Cursor.
type Viewport struct {
	Height int // visible rows (the pane's fixed height)
	Total  int // total content lines
	Offset int // index of the first visible line
	Cursor int // selected line (cursor-follow mode)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// maxOffset is the largest first-line index that still fills the pane.
func (v Viewport) maxOffset() int {
	if v.Total <= v.Height {
		return 0
	}
	return v.Total - v.Height
}

// follow scrolls the offset so the cursor is visible, then clamps everything.
func (v *Viewport) follow() {
	v.Cursor = clamp(v.Cursor, 0, max0(v.Total-1))
	if v.Height > 0 {
		if v.Cursor < v.Offset {
			v.Offset = v.Cursor
		} else if v.Cursor >= v.Offset+v.Height {
			v.Offset = v.Cursor - v.Height + 1
		}
	}
	v.Offset = clamp(v.Offset, 0, v.maxOffset())
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// --- cursor-follow mode (selectors) ---

// SetCursor places the cursor at c, clamps it, and scrolls to keep it visible.
func (v *Viewport) SetCursor(c int) { v.Cursor = c; v.follow() }

// Up/Down move the cursor one line, scrolling to keep it visible.
func (v *Viewport) Up()   { v.Cursor--; v.follow() }
func (v *Viewport) Down() { v.Cursor++; v.follow() }

// Top/Bottom jump the cursor to the first/last line.
func (v *Viewport) Top()    { v.Cursor = 0; v.follow() }
func (v *Viewport) Bottom() { v.Cursor = max0(v.Total - 1); v.follow() }

// PageUp/PageDown move the cursor by a (near) page.
func (v *Viewport) PageUp()   { v.Cursor -= v.page(); v.follow() }
func (v *Viewport) PageDown() { v.Cursor += v.page(); v.follow() }

func (v Viewport) page() int {
	if v.Height > 1 {
		return v.Height - 1
	}
	return 1
}

// --- free-scroll mode (detail panes) ---

// ScrollUp/ScrollDown move the window one line without a cursor.
func (v *Viewport) ScrollUp()   { v.Offset = clamp(v.Offset-1, 0, v.maxOffset()) }
func (v *Viewport) ScrollDown() { v.Offset = clamp(v.Offset+1, 0, v.maxOffset()) }

// ScrollPageUp/ScrollPageDown move the window by a page.
func (v *Viewport) ScrollPageUp()   { v.Offset = clamp(v.Offset-v.page(), 0, v.maxOffset()) }
func (v *Viewport) ScrollPageDown() { v.Offset = clamp(v.Offset+v.page(), 0, v.maxOffset()) }

// ScrollTop/ScrollBottom jump the window to the start/end.
func (v *Viewport) ScrollTop()    { v.Offset = 0 }
func (v *Viewport) ScrollBottom() { v.Offset = v.maxOffset() }

// --- resize / content change ---

// SetHeight assigns a new pane height and re-clamps.
func (v *Viewport) SetHeight(h int) {
	v.Height = max0(h)
	v.follow()
}

// SetTotal updates the content length and re-clamps cursor + offset.
func (v *Viewport) SetTotal(n int) {
	v.Total = max0(n)
	v.follow()
}

// Resize sets height and total and re-clamps Offset, preserving the scroll
// position — for Offset-driven panes (the detail view) that scroll by Offset
// rather than a cursor, so a re-layout doesn't snap them back to the top.
func (v *Viewport) Resize(height, total int) {
	v.Height = max0(height)
	v.Total = max0(total)
	v.Offset = clamp(v.Offset, 0, v.maxOffset())
}

// --- rendering ---

// Window returns the visible line range [start, end) clamped to Total.
func (v Viewport) Window() (start, end int) {
	start = clamp(v.Offset, 0, v.maxOffset())
	end = start + v.Height
	if end > v.Total {
		end = v.Total
	}
	if end < start {
		end = start
	}
	return start, end
}

// Render clips lines to the visible window and pads to exactly Height rows, so
// the pane is always its full height regardless of content length. Width/styling
// is the caller's concern.
func (v Viewport) Render(lines []string) string {
	start, end := v.Window()
	if start > len(lines) {
		start = len(lines)
	}
	if end > len(lines) {
		end = len(lines)
	}
	out := make([]string, 0, v.Height)
	out = append(out, lines[start:end]...)
	for len(out) < v.Height {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}
