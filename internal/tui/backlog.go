// package: tui / backlog
// type:    ui
// job:     builds the backlog list rows from []issue.Issue (task/spec rows, PR
//          sub-rows, gate rows) and renders the scrolling list with a cursor.
// limits:  no domain rules (-> issue), no color logic (-> render).
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/issue"
	"github.com/flo-at/sindri/internal/render"
)

// typeColCells is the fixed visual-cell width of the leftmost type/indent
// column. 6 cells cover depth 0–2 cleanly: depth 0 = icon (2 cells); depth 1 =
// "↳ " (2) + icon (2) = 4; depth 2 = "  ↳ " (4) + icon (2) = 6. Deeper trees
// overflow rather than truncate.
const typeColCells = 6

// statusColCells is the visual-cell width of the status column. Wide enough
// for "⚠ in_progress" (14 cells) or "🔨 some-dwarf-name"; longer values overflow
// into the title rather than truncate.
const statusColCells = 16

// typePrefix returns the leftmost-column content for an Issue: depth indent +
// arrow on non-root rows, followed by the type icon. Spec-only rows (no Task)
// have no icon and produce an empty content string; padCell turns that into
// pure space so the next column stays aligned.
func typePrefix(iss issue.Issue) string {
	var b strings.Builder
	if iss.Depth > 0 {
		b.WriteString(strings.Repeat("  ", iss.Depth-1))
		b.WriteString("↳ ")
	}
	if t := iss.Task; t != nil {
		b.WriteString(render.TaskTypeIcon(t.Type))
	}
	return b.String()
}

// padCell appends trailing spaces so s reaches n *visual* cells. The %-Ns verb
// in fmt counts bytes, not cells, which mis-aligns the column whenever an
// emoji (2 cells, multi-byte) shares a row with ASCII (1 cell each). lipgloss's
// Width is the same display width the terminal renders.
func padCell(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

type backlogRow struct {
	issueIdx int      // index into the issues slice
	isPR     bool     // true for a PR sub-row
	pr       issue.PR // set when isPR
	display  string
	plain    string // unstyled text for selection highlight
}

func buildBacklogRows(issues []issue.Issue) []backlogRow {
	var rows []backlogRow
	for ii, iss := range issues {
		title := iss.Title()
		if iss.IsClosed() {
			title = dimStyle.Render(title)
		}
		// The leftmost column carries the depth indent + arrow + type icon, so
		// every column after it stays aligned regardless of where in the tree
		// the row sits.
		typeCell := padCell(typePrefix(iss), typeColCells)
		tsStr := ""
		if !iss.UpdatedAt().IsZero() {
			tsStr = iss.UpdatedAt().Local().Format("06-01-02 15:04")
		}
		plain := fmt.Sprintf("%s %-9s %s  %s  %s  %s",
			typeCell,
			iss.ID(), iss.Priority(), tsStr,
			padCell(iss.Status(), statusColCells), iss.Title())
		line := fmt.Sprintf("%s %s %s  %s  %s  %s",
			typeCell,
			dimStyle.Render(fmt.Sprintf("%-9s", iss.ID())),
			dimStyle.Render(iss.Priority()),
			dimStyle.Render(tsStr),
			padCell(render.IssueStatus(iss), statusColCells),
			title,
		)
		rows = append(rows, backlogRow{issueIdx: ii, display: line, plain: plain})

		for _, pr := range iss.PRs {
			prPlain := fmt.Sprintf("    └ %s [%s]", pr.ID, pr.Status)
			prLine := fmt.Sprintf("    └ %s [%s]", pr.ID, render.PRStatus(pr.Status, iss.IsClosed()))
			rows = append(rows, backlogRow{issueIdx: ii, isPR: true, pr: pr, display: prLine, plain: prPlain})
		}

		if gates := render.Gates(iss.Gates()); gates != "" {
			rows = append(rows, backlogRow{issueIdx: ii, display: dimStyle.Render("    " + gates), plain: "    " + gates})
		}
	}
	return rows
}

func renderBacklogList(rows []backlogRow, cursor int, active, loaded bool) string {
	var b strings.Builder
	for i, row := range rows {
		if active && i == cursor {
			b.WriteString(selectedItemStyle.Render("> " + row.plain))
		} else {
			b.WriteString("  " + row.display)
		}
		b.WriteByte('\n')
	}
	if len(rows) == 0 {
		if !loaded {
			b.WriteString(dimStyle.Render("  Loading tasks…"))
		} else {
			b.WriteString(dimStyle.Render("  No tasks or PRs"))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

