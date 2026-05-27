package tui

import (
	"fmt"
	"strings"

	"github.com/flo-at/sindri/internal/worker"
)

type detailKind int

const (
	detailNone detailKind = iota
	detailTask
	detailPR
	detailWorker
)

type detailState struct {
	kind    detailKind
	content string
	scroll  int
}

func renderDetail(d detailState, width, height int) string {
	style := columnStyle.Width(width)

	// Hard budget: header takes 2 lines (title + blank), scroll indicator 1
	maxContent := height - 3
	if maxContent < 1 {
		maxContent = 1
	}

	var out []string
	out = append(out, headerStyle.Render("Detail"))

	if d.kind == detailNone {
		out = append(out, dimStyle.Render("  Select an item to view details"))
	} else {
		lines := strings.Split(d.content, "\n")
		start := d.scroll
		if start > len(lines) {
			start = len(lines)
		}

		// Show scroll-up indicator if scrolled
		if start > 0 {
			out = append(out, dimStyle.Render(fmt.Sprintf("  ↑ %d lines above", start)))
			maxContent--
		}

		// Reserve 1 line for overflow indicator if needed
		end := start + maxContent
		hasMore := end < len(lines)
		if hasMore {
			end = start + maxContent - 1
		}
		if end > len(lines) {
			end = len(lines)
		}

		for _, line := range lines[start:end] {
			out = append(out, "  "+line)
		}

		if hasMore {
			out = append(out, dimStyle.Render(fmt.Sprintf("  ↓ %d more (Shift+J/K)", len(lines)-end)))
		}
	}

	rendered := style.Render(strings.Join(out, "\n"))
	return clipHeight(rendered, height)
}

func (d *detailState) scrollDown(height int) {
	maxContent := height - 3
	if maxContent < 1 {
		maxContent = 1
	}
	lines := strings.Count(d.content, "\n") + 1
	maxScroll := lines - maxContent
	if maxScroll < 0 {
		maxScroll = 0
	}
	if d.scroll < maxScroll {
		d.scroll++
	}
}

func (d *detailState) scrollUp() {
	if d.scroll > 0 {
		d.scroll--
	}
}

func taskDetail(t taskItem, projectRoot string) detailState {
	return detailState{kind: detailTask, content: fetchTaskDetail(projectRoot, t.ID)}
}

func prDetail(pr prItem) detailState {
	var b strings.Builder
	fmt.Fprintf(&b, "PR:     %s\n", pr.ID)
	fmt.Fprintf(&b, "Title:  %s\n", pr.Title)
	fmt.Fprintf(&b, "Branch: %s → %s\n", pr.Branch, pr.Base)
	fmt.Fprintf(&b, "Status: %s\n", statusStyle(pr.Status))
	return detailState{kind: detailPR, content: b.String()}
}

func workerDetail(wk worker.Worker) detailState {
	var b strings.Builder
	fmt.Fprintf(&b, "Name:      %s\n", wk.Name)
	fmt.Fprintf(&b, "Role:      %s\n", wk.Role)
	fmt.Fprintf(&b, "Status:    %s\n", statusStyle(wk.Status))
	if wk.Container != "" {
		fmt.Fprintf(&b, "Container: %s\n", wk.Container)
	}
	if wk.Path != "" {
		fmt.Fprintf(&b, "Path:      %s\n", wk.Path)
	}
	if wk.Task != "" {
		fmt.Fprintf(&b, "Task:      %s\n", wk.Task)
	}
	if wk.PR != "" {
		fmt.Fprintf(&b, "PR:        %s\n", wk.PR)
	}
	return detailState{kind: detailWorker, content: b.String()}
}
