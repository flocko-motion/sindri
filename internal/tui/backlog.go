package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var dividerStyle = lipgloss.NewStyle().
	Foreground(subtle).
	PaddingTop(1).
	PaddingBottom(1)

func renderBacklogSplit(tasks []taskItem, prs []prItem, taskCursor, prCursor, activePanel int, width, height int, active bool) string {
	style := columnStyle.Width(width)
	if active {
		style = activeColumnStyle.Width(width)
	}

	contentWidth := width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Split height: tasks get upper half, PRs get lower half
	taskHeight := (height - 3) / 2
	prHeight := height - 3 - taskHeight - 1 // -1 for divider

	var b strings.Builder

	// Tasks section
	taskHeader := "Tasks"
	if active && activePanel == panelTasks {
		taskHeader = "Tasks ●"
	}
	b.WriteString(headerStyle.Render(taskHeader))
	b.WriteByte('\n')

	if len(tasks) == 0 {
		b.WriteString(dimStyle.Render("  No tasks"))
		b.WriteByte('\n')
	} else {
		rendered := 0
		for i, t := range tasks {
			if rendered >= taskHeight {
				b.WriteString(dimStyle.Render(fmt.Sprintf("  … %d more", len(tasks)-rendered)))
				b.WriteByte('\n')
				break
			}
			line := formatTask(t, contentWidth)
			if active && activePanel == panelTasks && i == taskCursor {
				b.WriteString(selectedItemStyle.Render("> " + line))
			} else {
				b.WriteString(normalItemStyle.Render("  " + line))
			}
			b.WriteByte('\n')
			rendered++
		}
	}

	// Divider
	divider := strings.Repeat("─", contentWidth)
	b.WriteString(dividerStyle.Render("  " + divider))
	b.WriteByte('\n')

	// PRs section
	prHeader := "Pull Requests"
	if active && activePanel == panelPRs {
		prHeader = "Pull Requests ●"
	}
	b.WriteString(headerStyle.Render(prHeader))
	b.WriteByte('\n')

	if len(prs) == 0 {
		b.WriteString(dimStyle.Render("  No PRs"))
		b.WriteByte('\n')
	} else {
		rendered := 0
		for i, pr := range prs {
			if rendered >= prHeight {
				b.WriteString(dimStyle.Render(fmt.Sprintf("  … %d more", len(prs)-rendered)))
				b.WriteByte('\n')
				break
			}
			line := formatPR(pr, contentWidth)
			if active && activePanel == panelPRs && i == prCursor {
				b.WriteString(selectedItemStyle.Render("> " + line))
			} else {
				b.WriteString(normalItemStyle.Render("  " + line))
			}
			b.WriteByte('\n')
			rendered++
		}
	}

	return style.Height(height).Render(b.String())
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
	case "in_review":
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
