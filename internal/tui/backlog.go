package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

func formatTaskLine(t taskItem) string {
	status := statusStyle(t.Status)
	prio := dimStyle.Render(t.Priority)
	return fmt.Sprintf("%s %s %s", prio, status, t.Title)
}

func formatPRLine(pr prItem) string {
	status := statusStyle(pr.Status)
	title := pr.Title
	if title == "" {
		title = pr.ID
	}
	return fmt.Sprintf("%s %s", status, title)
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
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

// unused but keep for lipgloss width calculations
var _ = lipgloss.Width
