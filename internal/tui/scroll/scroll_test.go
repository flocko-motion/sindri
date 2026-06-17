package scroll

import (
	"strings"
	"testing"
)

func lines(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = string(rune('a' + i))
	}
	return out
}

// Content shorter than the pane: no scroll, padded to full height.
func TestShorterThanPaneIsPadded(t *testing.T) {
	v := Viewport{Height: 5, Total: 2}
	out := v.Render([]string{"x", "y"})
	got := strings.Split(out, "\n")
	if len(got) != 5 {
		t.Fatalf("want 5 rows, got %d: %q", len(got), got)
	}
	if got[0] != "x" || got[1] != "y" || got[2] != "" || got[4] != "" {
		t.Fatalf("unexpected padded render: %q", got)
	}
	if v.maxOffset() != 0 {
		t.Fatalf("short content must not scroll")
	}
}

// Content longer than the pane: window of Height rows, scrolls with the cursor.
func TestLongerThanPaneScrollsWithCursor(t *testing.T) {
	v := Viewport{Height: 3, Total: 10}
	// Cursor at 0 → window [0,3).
	if s, e := v.Window(); s != 0 || e != 3 {
		t.Fatalf("initial window %d,%d", s, e)
	}
	// Move cursor past the bottom edge → window follows.
	for i := 0; i < 4; i++ {
		v.Down() // cursor → 4
	}
	if v.Cursor != 4 {
		t.Fatalf("cursor=%d", v.Cursor)
	}
	s, e := v.Window()
	if e-s != 3 || v.Cursor < s || v.Cursor >= e {
		t.Fatalf("cursor not in window: cur=%d window=[%d,%d)", v.Cursor, s, e)
	}
	out := strings.Split(v.Render(lines(10)), "\n")
	if len(out) != 3 {
		t.Fatalf("want 3 rows, got %d", len(out))
	}
}

func TestCursorClampsAtEdges(t *testing.T) {
	v := Viewport{Height: 3, Total: 5}
	v.Up() // already at top
	if v.Cursor != 0 || v.Offset != 0 {
		t.Fatalf("top edge: cur=%d off=%d", v.Cursor, v.Offset)
	}
	v.Bottom()
	if v.Cursor != 4 {
		t.Fatalf("bottom cursor=%d", v.Cursor)
	}
	v.Down() // past bottom
	if v.Cursor != 4 {
		t.Fatalf("cursor moved past end: %d", v.Cursor)
	}
	if v.Offset != v.maxOffset() {
		t.Fatalf("offset not at max: %d vs %d", v.Offset, v.maxOffset())
	}
}

// Resizing the pane below the cursor must pull the window down to keep it visible.
func TestResizeBelowCursor(t *testing.T) {
	v := Viewport{Height: 10, Total: 20}
	v.Bottom() // cursor 19, offset 10
	v.SetHeight(3)
	s, e := v.Window()
	if v.Cursor < s || v.Cursor >= e {
		t.Fatalf("cursor lost after resize: cur=%d window=[%d,%d)", v.Cursor, s, e)
	}
	if e-s != 3 {
		t.Fatalf("window not resized: %d..%d", s, e)
	}
}

// Content shrinking below the offset must pull cursor + offset back in range.
func TestContentShrinks(t *testing.T) {
	v := Viewport{Height: 3, Total: 20}
	v.Bottom() // cursor 19
	v.SetTotal(4)
	if v.Cursor != 3 {
		t.Fatalf("cursor not clamped to new total: %d", v.Cursor)
	}
	if v.Offset > v.maxOffset() {
		t.Fatalf("offset beyond content: off=%d max=%d", v.Offset, v.maxOffset())
	}
	s, e := v.Window()
	if v.Cursor < s || v.Cursor >= e {
		t.Fatalf("cursor not visible after shrink")
	}
}

func TestFreeScrollClamps(t *testing.T) {
	v := Viewport{Height: 4, Total: 10}
	v.ScrollUp() // already top
	if v.Offset != 0 {
		t.Fatalf("scrolled above top: %d", v.Offset)
	}
	v.ScrollBottom()
	if v.Offset != v.maxOffset() || v.Offset != 6 {
		t.Fatalf("scroll bottom offset=%d (max %d)", v.Offset, v.maxOffset())
	}
	v.ScrollDown() // past bottom
	if v.Offset != 6 {
		t.Fatalf("scrolled past bottom: %d", v.Offset)
	}
}

func TestEmptyContent(t *testing.T) {
	v := Viewport{Height: 4, Total: 0}
	v.Down()
	v.Bottom()
	s, e := v.Window()
	if s != 0 || e != 0 {
		t.Fatalf("empty window %d,%d", s, e)
	}
	out := strings.Split(v.Render(nil), "\n")
	if len(out) != 4 {
		t.Fatalf("empty render should pad to height, got %d", len(out))
	}
}
