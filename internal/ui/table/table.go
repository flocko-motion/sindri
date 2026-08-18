// package: ui/table / table
// type:    rendering (shared column layout)
// job:     one column layout per list, producing BOTH its label line and its rows,
// so a header cannot drift from the columns it names — and giving the CLI
// and the TUI one way to lay a table out instead of two.
// limits:  widths and padding only; the colours are the caller's (a Cell carries its
// own style) and which rows a list shows belongs to the front-end.
package table

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Column is one column: the label over it, the display cells it occupies, and whether its content is
// right-aligned. A Width of 0 marks the last column, which takes the rest of the line unpadded — a
// title, a branch, a command, a message body.
type Column struct {
	Label string
	Width int
	Right bool
	// Clip cuts a value too wide for the column, marking it so the reader knows it was cut. Set it
	// where the values are unbounded and a long one would push every column after it out of line — a
	// repo name. Left off, the column pads and an over-long value takes the room it needs: an id or a
	// status word half-shown is worse than one row sitting wide.
	Clip bool
}

// Fit lays s out in the column: padded to its width, or cut where the column clips. Width 0 is the
// last column, which is returned whole.
func (c Column) Fit(s string) string {
	if c.Width <= 0 {
		return s
	}
	w := ansi.StringWidth(s)
	if w > c.Width {
		if !c.Clip {
			return s
		}
		return ansi.Truncate(s, c.Width, "…")
	}
	pad := strings.Repeat(" ", c.Width-w)
	if c.Right {
		return pad + s
	}
	return s + pad
}

// Table is a list's columns, left to right. One value per list, read by both the header and every
// row: a header laid out from numbers of its own is aligned the day it is written and drifts the
// first time a column moves or a glyph changes width.
type Table []Column

// Cell is one column's content and the style to render it in. The style is applied AFTER padding,
// since its escape sequences would otherwise be counted as the content's width. A nil Style is plain
// text, which is what most of the CLI's cells are.
type Cell struct {
	Text  string
	Style func(...string) string
}

// Header is the label line, laid out through the very columns the rows use. Unstyled: whether it is
// dimmed as chrome is the front-end's to decide, and both do.
func (t Table) Header() string {
	cells := make([]Cell, len(t))
	for i, c := range t {
		cells[i] = Cell{Text: c.Label}
	}
	return t.Line(cells...)
}

// Line lays cells out left to right, one space between columns. A cell beyond the last column is
// appended whole rather than dropped, so a row that carries more than the layout describes is
// visible instead of silently short.
func (t Table) Line(cells ...Cell) string {
	out := make([]string, len(cells))
	for i, c := range cells {
		text := c.Text
		if i < len(t) {
			text = t[i].Fit(text)
		}
		if c.Style != nil {
			text = c.Style(text)
		}
		out[i] = text
	}
	return strings.Join(out, " ")
}
