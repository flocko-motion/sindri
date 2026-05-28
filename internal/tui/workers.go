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

		task := wk.Task
		if task == "" {
			task = "-"
		}
		pr := wk.PR
		if pr == "" {
			pr = "-"
		}
		path := "-"
		if wk.Path != "" {
			path = filepath.Base(wk.Path)
		}
		branch := wk.Branch
		if branch == "" {
			branch = "-"
		}

		plain := fmt.Sprintf("%s %-12s %-8s %-30s %-18s %-12s %s",
			icon, wk.Name, wk.Status, task, pr, path, branch)

		rows = append(rows, row{
			icon:   icon,
			name:   wk.Name,
			status: wk.Status,
			task:   task,
			pr:     pr,
			path:   path,
			branch: branch,
			plain:  plain,
		})
	}

	for i, r := range rows {
		if active && i == cursor {
			b.WriteString(selectedItemStyle.Render("> " + r.plain))
		} else {
			line := fmt.Sprintf("  %s %-12s %s  %-30s %s  %-12s %s",
				r.icon,
				r.name,
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
