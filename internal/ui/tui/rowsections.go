// package: tui / rowsections
// type:    ui (grouped selector lists)
// job:     label the two groups a scoped list can hold — the rows from another repo
// that wait on the user, and the rows of the repo in view — as rows that
// select nothing, so the cursor walks past them.
// limits:  assembling and labelling only; each row's own rendering is its tab's
// (-> tab_agents.go, tab_prs.go), and which group a row belongs in is the
// scope rule's (-> items.go inScope).
package tui

import "github.com/flo-at/sindri/internal/api"

// headingRow is a section label in the row list. It selects nothing, so j/k step over it and the
// detail pane is never asked to show a heading (-> moveCursor).
func headingRow(text string) row { return row{text: text} }

// spacerRow is the blank line between two sections, a row for the same reason a heading is one.
func spacerRow() row { return row{} }

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
