package table

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// demo is a layout with each kind of column: fixed, right-aligned, and the trailing one.
var demo = Table{
	{Label: "repo", Width: 6},
	{Label: "age", Width: 4, Right: true},
	{Label: "title"},
}

// TestTheHeaderSitsOverItsColumns is the whole point of one layout doing both jobs: each label starts
// in the same cell as the values beneath it. A header built from its own widths agrees on the day it
// is written and drifts the first time a column moves.
func TestTheHeaderSitsOverItsColumns(t *testing.T) {
	header := demo.Header()
	row := demo.Line(Cell{Text: "sindri"}, Cell{Text: "2d"}, Cell{Text: "a title"})
	// A left-aligned column reads down its first cell, so its label starts where its values do.
	for _, c := range []struct{ label, value string }{{"repo", "sindri"}, {"title", "a title"}} {
		at, under := offsetOf(header, c.label), offsetOf(row, c.value)
		if at < 0 || under < 0 {
			t.Fatalf("%q / %q: not found in\n%q\n%q", c.label, c.value, header, row)
		}
		if at != under {
			t.Errorf("label %q starts at cell %d, its values at %d:\n%q\n%q", c.label, at, under, header, row)
		}
	}
}

// offsetOf is where s begins in line, measured in display cells, or -1 when it is absent.
func offsetOf(line, s string) int {
	i := strings.Index(line, s)
	if i < 0 {
		return -1
	}
	return ansi.StringWidth(line[:i])
}

// TestARightAlignedColumnEndsWhereItsLabelDoes: a number column reads down its last digit, so the
// label belongs over that end rather than over its start.
func TestARightAlignedColumnEndsWhereItsLabelDoes(t *testing.T) {
	line := demo.Line(Cell{Text: "sindri"}, Cell{Text: "2d"}, Cell{Text: "x"})
	if !strings.Contains(line, "sindri   2d x") {
		t.Errorf("the age cell should be right-aligned in its 4 cells: %q", line)
	}
	if !strings.Contains(demo.Header(), "repo    age title") {
		t.Errorf("the label should be right-aligned too: %q", demo.Header())
	}
}

// TestTheLastColumnIsNeverPadded: it takes the rest of the line, so padding it would trail spaces
// into every row for nothing.
func TestTheLastColumnIsNeverPadded(t *testing.T) {
	line := demo.Line(Cell{Text: "a"}, Cell{Text: "b"}, Cell{Text: "the title"})
	if strings.HasSuffix(line, " ") {
		t.Errorf("the trailing column padded itself: %q", line)
	}
}

// TestAClippingColumnKeepsItsWidth: where the values are unbounded — a repo name — one long one
// would push every column after it out of line, and a header makes that crookedness plain.
func TestAClippingColumnKeepsItsWidth(t *testing.T) {
	clipped := Table{{Label: "repo", Width: 6, Clip: true}, {Label: "title"}}
	line := clipped.Line(Cell{Text: "averylongreponame"}, Cell{Text: "x"})
	if got := ansi.StringWidth(line); got != 6+1+1 {
		t.Errorf("the row is %d cells wide, so a long value escaped its column: %q", got, line)
	}
	if !strings.Contains(line, "…") {
		t.Errorf("a clipped value should say it was clipped: %q", line)
	}
}

// TestAPlainColumnKeepsTheWholeValue: half an id cannot be copied and half a status word cannot be
// read, so a column that does not ask to clip takes the room it needs and lets that one row sit wide.
func TestAPlainColumnKeepsTheWholeValue(t *testing.T) {
	line := demo.Line(Cell{Text: "averylongreponame"}, Cell{Text: "2d"}, Cell{Text: "x"})
	if !strings.Contains(line, "averylongreponame") {
		t.Errorf("the value was cut: %q", line)
	}
}

// TestStyleIsAppliedAfterPadding: a style applied first would have its escape sequences counted as
// content, and every cell after it would sit a few columns early.
func TestStyleIsAppliedAfterPadding(t *testing.T) {
	bold := func(s ...string) string { return "\x1b[1m" + strings.Join(s, "") + "\x1b[0m" }
	styled := demo.Line(Cell{Text: "sin", Style: bold}, Cell{Text: "2d"}, Cell{Text: "x"})
	plain := demo.Line(Cell{Text: "sin"}, Cell{Text: "2d"}, Cell{Text: "x"})
	if ansi.StringWidth(styled) != ansi.StringWidth(plain) {
		t.Errorf("styling changed the layout:\n%q\n%q", styled, plain)
	}
}

// TestFitCountsDisplayCells, not runes or bytes: a marker column is padded from a glyph's drawn
// width, and a two-cell glyph counted as one rune would knock every later column out of line.
func TestFitCountsDisplayCells(t *testing.T) {
	col := Column{Width: 6}
	for _, str := range []string{"ab", "\u251c\u2500\u25be", "\uf0ad\uf407"} {
		if got := ansi.StringWidth(col.Fit(str)); got != 6 {
			t.Errorf("Fit(%q) is %d cells wide, want 6", str, got)
		}
	}
}
