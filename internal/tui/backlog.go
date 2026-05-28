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
		if iss.IsClosed() {
			title = dimStyle.Render(title)
		}
		tsStr := ""
		if !iss.UpdatedAt().IsZero() {
			tsStr = iss.UpdatedAt().Local().Format("06-01-02 15:04")
		}
		plain := fmt.Sprintf("%-9s %s  %s  %s  %s", iss.ID(), iss.Priority(), tsStr, iss.Status(), iss.Title())
		line := fmt.Sprintf("%s %s  %s  %s  %s",
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

func renderBacklogList(rows []backlogRow, cursor int, active bool) string {
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
		b.WriteString(dimStyle.Render("  No tasks or PRs"))
		b.WriteByte('\n')
	}
	return b.String()
}

