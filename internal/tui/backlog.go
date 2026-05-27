package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type backlogRow struct {
	isPR     bool
	taskIdx  int
	prIdx    int
	display  string
}

func buildBacklogRows(tasks []taskItem, prs []prItem, workersByTask map[string]string) []backlogRow {
	prByTask := make(map[string][]int)
	var orphanPRs []int
	for i, pr := range prs {
		taskID := extractTaskIDFromTitle(pr.Title)
		if taskID != "" {
			prByTask[taskID] = append(prByTask[taskID], i)
		} else {
			orphanPRs = append(orphanPRs, i)
		}
	}

	var rows []backlogRow
	for ti, t := range tasks {
		status := statusStyle(t.Status)
		if w, ok := workersByTask[t.ID]; ok && t.Status == "in_progress" {
			status = statusRunning.Render("🔨 " + w)
		}
		title := t.Title
		if t.Status == "closed" || t.Status == "approved" {
			title = dimStyle.Render(t.Title)
		}
		line := fmt.Sprintf("%s  %s  %s",
			dimStyle.Render(t.Priority),
			status,
			title,
		)
		rows = append(rows, backlogRow{taskIdx: ti, display: line})

		for _, pi := range prByTask[t.ID] {
			pr := prs[pi]
			prLine := fmt.Sprintf("    └ %s [%s]", pr.ID, pr.Status)
			rows = append(rows, backlogRow{isPR: true, prIdx: pi, display: prLine})
		}

		if gates := renderGates(t.Labels); gates != "" {
			rows = append(rows, backlogRow{taskIdx: ti, display: dimStyle.Render("    " + gates)})
		}
	}

	for _, pi := range orphanPRs {
		pr := prs[pi]
		prLine := fmt.Sprintf("%s  %s", statusStyle(pr.Status), pr.Title)
		if pr.Title == "" {
			prLine = fmt.Sprintf("%s  %s", statusStyle(pr.Status), pr.ID)
		}
		rows = append(rows, backlogRow{isPR: true, prIdx: pi, display: prLine})
	}

	return rows
}

func renderBacklogList(rows []backlogRow, cursor int, active bool) string {
	var b strings.Builder
	for i, row := range rows {
		if active && i == cursor {
			b.WriteString(selectedItemStyle.Render("> " + row.display))
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

func statusStyle(status string) string {
	switch status {
	case "in_progress", "running":
		return statusRunning.Render(status)
	case "open":
		return statusOpen.Render(status)
	case "in_review":
		return statusOpen.Render(status)
	case "merged", "approved", "closed":
		return statusDone.Render(status)
	default:
		return dimStyle.Render(status)
	}
}

func extractTaskIDFromTitle(title string) string {
	if m := taskIDRe.FindStringSubmatch(title); len(m) > 1 {
		return m[1]
	}
	return ""
}

func renderGates(labels []string) string {
	approved := make(map[string]bool)
	var required []string
	for _, l := range labels {
		if strings.HasPrefix(l, "require-review-") {
			required = append(required, strings.TrimPrefix(l, "require-review-"))
		}
		if strings.HasPrefix(l, "approved-review-") {
			approved[strings.TrimPrefix(l, "approved-review-")] = true
		}
	}
	if len(required) == 0 {
		return ""
	}
	var parts []string
	for _, r := range required {
		if approved[r] {
			parts = append(parts, "☑ "+r)
		} else {
			parts = append(parts, "☐ "+r)
		}
	}
	return strings.Join(parts, "  ")
}

var _ = lipgloss.Width // keep import
