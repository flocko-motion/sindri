// package: tui / workers
// type:    ui
// job:     renders the workers view — each worker's status, task, branch, path.
// limits:  presentation only; worker data comes from internal/worker.
package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/render"
	"github.com/flo-at/sindri/internal/worker"
)

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func renderWorkersList(workers []worker.Worker, cursor int, active bool) string {
	var b strings.Builder
	if len(workers) == 0 {
		b.WriteString(dimStyle.Render("  No workers"))
		b.WriteByte('\n')
		return b.String()
	}

	type row struct {
		icon   string
		name   string
		role   string
		status string
		task   string
		pr     string
		path   string
		branch string
		plain  string
	}

	var rows []row
	for _, wk := range workers {
		icon := "🔨"
		if wk.IsMain {
			icon = "👑"
		} else if wk.Role == "orphan" {
			icon = "⚠ "
		}

		path := "-"
		if wk.Path != "" {
			path = filepath.Base(wk.Path)
		}

		plain := fmt.Sprintf("%s %-12s %-9s %-8s %-26s %-16s %-12s %s",
			icon, wk.Name, wk.Role, wk.Status, dash(wk.Task), dash(wk.PR), path, dash(wk.Branch))

		rows = append(rows, row{
			icon:   icon,
			name:   wk.Name,
			role:   wk.Role,
			status: wk.Status,
			task:   dash(wk.Task),
			pr:     dash(wk.PR),
			path:   path,
			branch: dash(wk.Branch),
			plain:  plain,
		})
	}

	for i, r := range rows {
		if active && i == cursor {
			b.WriteString(selectedItemStyle.Render("> " + r.plain))
		} else {
			line := fmt.Sprintf("  %s %-12s %-9s %s  %-26s %s  %-12s %s",
				r.icon,
				r.name,
				dimStyle.Render(r.role),
				render.TaskStatus(r.status),
				dimStyle.Render(r.task),
				dimStyle.Render(r.pr),
				dimStyle.Render(r.path),
				dimStyle.Render(r.branch),
			)
			b.WriteString(line)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
