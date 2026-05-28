package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func listModel(width, height, nrows int) Model {
	m := New(".")
	m.width = width
	m.height = height
	m.resizeViewports()
	rows := make([]backlogRow, nrows)
	for i := range rows {
		txt := fmt.Sprintf("td-%06d  P2  26-05-28 10:00  open  Task number %d", i, i)
		rows[i] = backlogRow{taskIdx: i, display: txt, plain: txt}
	}
	m.backlogRows = rows
	return m
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Every rendered line must fit within the terminal width, otherwise the
// terminal wraps it and the right border is clipped / layout overflows.
func TestColumnFitsWidth(t *testing.T) {
	for _, w := range []int{80, 100, 120} {
		m := listModel(w, 24, 50)
		out := m.viewList()
		for i, line := range strings.Split(out, "\n") {
			// Skip the title bar (line 0): its long help text is separate chrome.
			if i == 0 {
				continue
			}
			if lw := lipgloss.Width(line); lw > w {
				t.Errorf("width=%d line %d overflows: %d cols: %q", w, i, lw, stripANSI(line))
			}
		}
	}
}

func TestRightBorderPresent(t *testing.T) {
	m := listModel(100, 24, 50)
	out := m.viewList()
	lines := strings.Split(out, "\n")
	bordered := 0
	for _, line := range lines {
		plain := stripANSI(line)
		if strings.HasPrefix(plain, "│") || strings.HasPrefix(plain, "╭") || strings.HasPrefix(plain, "╰") {
			bordered++
			r := []rune(strings.TrimRight(plain, " "))
			last := r[len(r)-1]
			if last != '│' && last != '╮' && last != '╯' {
				t.Errorf("line missing right border: %q", plain)
			}
		}
	}
	if bordered == 0 {
		t.Fatal("no bordered lines found")
	}
}

// The selected row must remain visible no matter how far the cursor moves.
func TestSelectionStaysVisible(t *testing.T) {
	for _, target := range []int{0, 5, 16, 30, 49} {
		m := listModel(100, 24, 50)
		// Drive the cursor exactly as Update would, one step at a time.
		for m.listCursor < target {
			m.moveCursor(1)
		}
		out := m.viewList()
		want := fmt.Sprintf("> td-%06d", target)
		if !strings.Contains(stripANSI(out), want) {
			t.Errorf("cursor=%d not visible (YOffset=%d, h=%d)", target, m.vpList.YOffset, m.vpList.Height)
		}
	}
}

// Moving up after scrolling down keeps the selection visible and scrolls back.
func TestScrollBackUp(t *testing.T) {
	m := listModel(100, 24, 50)
	for m.listCursor < 49 {
		m.moveCursor(1)
	}
	for m.listCursor > 0 {
		m.moveCursor(-1)
		out := stripANSI(m.viewList())
		want := fmt.Sprintf("> td-%06d", m.listCursor)
		if !strings.Contains(out, want) {
			t.Fatalf("cursor=%d not visible on the way up (YOffset=%d)", m.listCursor, m.vpList.YOffset)
		}
	}
}
