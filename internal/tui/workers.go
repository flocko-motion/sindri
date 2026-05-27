package tui

import (
	"fmt"
	"strings"

	"github.com/flo-at/sindri/internal/worker"
)

func renderWorkers(workers []worker.Worker, selected int, width, height int, active bool) string {
	style := columnStyle.Width(width)
	if active {
		style = activeColumnStyle.Width(width)
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("Workers"))
	b.WriteByte('\n')

	if len(workers) == 0 {
		b.WriteString(dimStyle.Render("  No workers"))
		b.WriteByte('\n')
	}

	for i, wk := range workers {
		line := formatWorker(wk)
		if active && i == selected {
			b.WriteString(selectedItemStyle.Render("> " + line))
		} else {
			b.WriteString(normalItemStyle.Render("  " + line))
		}
		b.WriteByte('\n')
	}

	content := b.String()
	rendered := style.Render(content)
	return clipHeight(rendered, height)
}

func formatWorker(wk worker.Worker) string {
	icon := "🔨"
	if wk.IsMain {
		icon = "👑"
	} else if wk.Role == "orphan" {
		icon = "⚠ "
	}

	status := statusStyle(wk.Status)
	name := wk.Name

	parts := []string{fmt.Sprintf("%s %s %s", icon, name, status)}
	if wk.Task != "" {
		parts = append(parts, dimStyle.Render(wk.Task))
	}
	if wk.PR != "" {
		parts = append(parts, dimStyle.Render(wk.PR))
	}
	return strings.Join(parts, " ")
}
