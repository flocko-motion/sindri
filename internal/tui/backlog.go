// package: tui / backlog
// type:    ui
// job:     builds the backlog list rows from []issue.Issue (task/spec rows, PR
//          sub-rows, gate rows) and renders the scrolling list with a cursor.
// limits:  no domain rules (-> issue), no color logic (-> render).
package tui

import (
	"fmt"
	"strings"

	"github.com/flo-at/sindri/internal/issue"
	"github.com/flo-at/sindri/internal/render"
)

// typeColWidth is the fixed character count of the leftmost type/indent column.
// 6 chars cover depth 0–2 cleanly (depth 2 = "  ↳ " + icon ≈ 6 chars wide);
// deeper trees overflow into the id column rather than truncate.
const typeColWidth = 6

// typePrefix returns the leftmost-column content for an Issue: depth indent +
// arrow on non-root rows, followed by the type icon when there is one. Spec-only
// rows have neither and produce "", so the column becomes pure padding.
func typePrefix(iss issue.Issue) string {
	var b strings.Builder
	if iss.Depth > 0 {
		b.WriteString(strings.Repeat("  ", iss.Depth-1))
		b.WriteString("↳ ")
	}
	if t := iss.Task; t != nil {
		if icon := render.TaskTypeIcon(t.Type); icon != "" {
			b.WriteString(icon)
		}
	}
	return b.String()
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
		typeCell := typePrefix(iss)
		tsStr := ""
		if !iss.UpdatedAt().IsZero() {
			tsStr = iss.UpdatedAt().Local().Format("06-01-02 15:04")
		}
		plain := fmt.Sprintf("%-*s %-9s %s  %s  %s  %s",
			typeColWidth, typeCell,
			iss.ID(), iss.Priority(), tsStr, iss.Status(), iss.Title())
		line := fmt.Sprintf("%-*s %s %s  %s  %s  %s",
			typeColWidth, typeCell,
			dimStyle.Render(fmt.Sprintf("%-9s", iss.ID())),
			dimStyle.Render(iss.Priority()),
			dimStyle.Render(tsStr),
			render.IssueStatus(iss),
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

