package tui

import (
	"fmt"
	"strings"

	"github.com/flo-at/sindri/internal/worker"
)

func renderWorkersList(workers []worker.Worker, cursor int, active bool) string {
	var b strings.Builder
	if len(workers) == 0 {
		b.WriteString(dimStyle.Render("  No workers"))
		b.WriteByte('\n')
		return b.String()
	}
	for i, wk := range workers {
		line := formatWorkerLine(wk)
		if active && i == cursor {
			b.WriteString(selectedItemStyle.Render("> " + line))
		} else {
			b.WriteString("  " + line)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func formatWorkerLine(wk worker.Worker) string {
	icon := "🔨"
	if wk.IsMain {
		icon = "👑"
	} else if wk.Role == "orphan" {
		icon = "⚠ "
	}

	status := statusStyle(wk.Status)
	parts := []string{fmt.Sprintf("%s %s %s", icon, wk.Name, status)}
	if wk.Task != "" {
		parts = append(parts, dimStyle.Render(wk.Task))
	}
	if wk.PR != "" {
		parts = append(parts, dimStyle.Render(wk.PR))
	}
	return strings.Join(parts, " ")
}
