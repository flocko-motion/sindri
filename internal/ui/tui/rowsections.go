// package: tui / rowsections
// type:    ui (labelled selector lists)
// job:     the lines a selector list holds that are not rows — the column labels over
// it, and the headings over the two groups a scoped list can hold — each a
// row that selects nothing, so the cursor walks past them.
// limits:  assembling and labelling only; each row's own rendering is its tab's
// (-> tab_*.go), the widths are its table's (-> ui/table), and which group a
// row belongs in is the scope rule's (-> items.go inScope).
package tui

import (
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/table"
)

// headingRow is a label in the row list — column names, a section heading. It selects nothing, so
// j/k step over it and the detail pane is never asked to show a label (-> moveCursor).
func headingRow(text string) row { return row{text: text} }

// spacerRow is the blank line between two sections, a row for the same reason a heading is one.
func spacerRow() row { return row{} }

// listing is a tab's whole row list: the column labels, then its rows, sectioned when some of them
// come from another repo. One assembler, so no tab can forget its labels or invent a second way of
// putting an unselectable line in a list. Empty in, empty out — labels over no rows are chrome
// explaining a table that isn't there, and every tab's own empty state says more.
func listing(t table.Table, foreign, local []row) []row {
	rows := sectioned(foreign, local)
	if len(rows) == 0 {
		return nil
	}
	return append([]row{headingRow(dimStyle.Render(t.Header()))}, rows...)
}

// sectioned labels foreign rows above local ones, and leaves a purely local list exactly as it was.
// Foreign first because the only reason those rows are on screen is that they need the user; under
// the local list they would be back to being found by scrolling, which is what admitting them fixed.
func sectioned(foreign, local []row) []row {
	if len(foreign) == 0 {
		return local
	}
	out := make([]row, 0, len(foreign)+len(local)+3)
	out = append(out, headingRow(stCrit.Render(api.ForeignAttentionHeading(len(foreign)))))
	out = append(out, foreign...)
	out = append(out, spacerRow(), headingRow(dimStyle.Render(api.LocalHeading)))
	return append(out, local...)
}
