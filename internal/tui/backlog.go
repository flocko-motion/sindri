package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func renderBacklog(tasks []taskItem, prs []prItem, selected int, width, height int, active bool) string {
	style := columnStyle.Width(width)
	if active {
		style = activeColumnStyle.Width(width)
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("Backlog"))
	b.WriteByte('\n')

	contentWidth := width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	idx := 0
	if len(tasks) > 0 {
		b.WriteString(dimStyle.Render("  Tasks"))
		b.WriteByte('\n')
		for _, t := range tasks {
			line := formatTask(t, contentWidth)
			if active && idx == selected {
				b.WriteString(selectedItemStyle.Render("> " + line))
			} else {
				b.WriteString(normalItemStyle.Render("  " + line))
			}
			b.WriteByte('\n')
			idx++
		}
	}

	if len(prs) > 0 {
		b.WriteByte('\n')
		b.WriteString(dimStyle.Render("  Pull Requests"))
		b.WriteByte('\n')
		for _, pr := range prs {
			line := formatPR(pr, contentWidth)
			if active && idx == selected {
				b.WriteString(selectedItemStyle.Render("> " + line))
			} else {
				b.WriteString(normalItemStyle.Render("  " + line))
			}
			b.WriteByte('\n')
			idx++
		}
	}

	if len(tasks) == 0 && len(prs) == 0 {
		b.WriteString(dimStyle.Render("  No tasks or PRs"))
		b.WriteByte('\n')
	}

	content := b.String()
	return style.Height(height).Render(content)
}

func formatTask(t taskItem, width int) string {
	status := statusStyle(t.Status)
	prio := dimStyle.Render(t.Priority)
	prefix := fmt.Sprintf("%s %s ", prio, status)

	prefixLen := lipgloss.Width(prefix)
	titleWidth := width - prefixLen - 2
	if titleWidth < 5 {
		titleWidth = 5
	}
	title := truncate(t.Title, titleWidth)
	return prefix + title
}

func formatPR(pr prItem, width int) string {
	status := statusStyle(pr.Status)
	prefix := fmt.Sprintf("%s ", status)

	prefixLen := lipgloss.Width(prefix)
	titleWidth := width - prefixLen - 2
	if titleWidth < 5 {
		titleWidth = 5
	}
	title := pr.Title
	if title == "" {
		title = pr.ID
	}
	return prefix + truncate(title, titleWidth)
}

func statusStyle(status string) string {
	switch status {
	case "in_progress", "running":
		return statusRunning.Render(status)
	case "open":
		return statusOpen.Render(status)
	case "merged", "approved", "closed":
		return statusDone.Render(status)
	default:
		return dimStyle.Render(status)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func backlogItemCount(tasks []taskItem, prs []prItem) int {
	return len(tasks) + len(prs)
}
