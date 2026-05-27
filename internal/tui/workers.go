package tui

import (
	"fmt"
	"strings"

	"github.com/flo-at/sindri/internal/worker"
)

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
