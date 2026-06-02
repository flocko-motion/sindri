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
		if t := iss.Task; t != nil {
			if icon := render.TaskTypeIcon(t.Type); icon != "" {
				title = icon + " " + title
			}
		}
		if iss.IsClosed() {
			title = dimStyle.Render(title)
		}
		// Indent children under their parent: 2 spaces per depth level + an
		// arrow on every non-root row.
		indent := ""
		if iss.Depth > 0 {
			indent = strings.Repeat("  ", iss.Depth) + "↳ "
		}
		tsStr := ""
		if !iss.UpdatedAt().IsZero() {
			tsStr = iss.UpdatedAt().Local().Format("06-01-02 15:04")
		}
		plainTitle := iss.Title()
		if t := iss.Task; t != nil {
			if icon := render.TaskTypeIcon(t.Type); icon != "" {
				plainTitle = icon + " " + plainTitle
			}
		}
		plain := indent + fmt.Sprintf("%-9s %s  %s  %s  %s", iss.ID(), iss.Priority(), tsStr, iss.Status(), plainTitle)
		line := indent + fmt.Sprintf("%s %s  %s  %s  %s",
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

