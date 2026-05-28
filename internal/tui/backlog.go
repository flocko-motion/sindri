package tui

import (
	"fmt"
	"strings"

	"github.com/flo-at/sindri/internal/issue"
	"github.com/flo-at/sindri/internal/render"
)

type backlogRow struct {
	isPR    bool
	taskIdx int
	prIdx   int
	display string
	plain   string // unstyled text for selection highlight
}

func buildBacklogRows(tasks []taskItem, prs []prItem, workersByTask map[string]string) []backlogRow {
	prByTask := make(map[string][]int)
	var orphanPRs []int
	for i, pr := range prs {
		taskID := issue.TaskIDFromTitle(pr.Title)
		if taskID != "" {
			prByTask[taskID] = append(prByTask[taskID], i)
		} else {
			orphanPRs = append(orphanPRs, i)
		}
	}

	var rows []backlogRow
	for ti, t := range tasks {
		var status, statusText string
		if w, ok := workersByTask[t.ID]; ok {
			status = render.Worker(w)
			statusText = "🔨 " + w
		} else if t.Status == "in_progress" {
			status = render.Orphaned()
			statusText = "⚠ in_progress"
		} else {
			status = render.TaskStatus(t.Status)
			statusText = t.Status
		}
		title := t.Title
		if t.IsClosed() {
			title = dimStyle.Render(t.Title)
		}
		tsStr := ""
		if !t.UpdatedAt.IsZero() {
			tsStr = t.UpdatedAt.Local().Format("06-01-02 15:04")
		}
		plain := fmt.Sprintf("%-9s %s  %s  %s  %s", t.ID, t.Priority, tsStr, statusText, t.Title)
		line := fmt.Sprintf("%s %s  %s  %s  %s",
			dimStyle.Render(fmt.Sprintf("%-9s", t.ID)),
			dimStyle.Render(t.Priority),
			dimStyle.Render(tsStr),
			status,
			title,
		)
		rows = append(rows, backlogRow{taskIdx: ti, display: line, plain: plain})

		for _, pi := range prByTask[t.ID] {
			pr := prs[pi]
			prPlain := fmt.Sprintf("    └ %s [%s]", pr.ID, pr.Status)
			prLine := fmt.Sprintf("    └ %s [%s]", pr.ID, render.PRStatus(pr.Status, t.IsClosed()))
			rows = append(rows, backlogRow{isPR: true, prIdx: pi, display: prLine, plain: prPlain})
		}

		if gates := render.Gates(t.Gates()); gates != "" {
			rows = append(rows, backlogRow{taskIdx: ti, display: dimStyle.Render("    " + gates), plain: "    " + gates})
		}
	}

	for _, pi := range orphanPRs {
		pr := prs[pi]
		prTitle := pr.Title
		if prTitle == "" {
			prTitle = pr.ID
		}
		prLine := fmt.Sprintf("%s  %s", render.PRStatus(pr.Status, false), prTitle)
		prPlain := fmt.Sprintf("%s  %s", pr.Status, prTitle)
		rows = append(rows, backlogRow{isPR: true, prIdx: pi, display: prLine, plain: prPlain})
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

