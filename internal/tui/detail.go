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
}

func renderDetail(d detailState, width, height int) string {
	style := columnStyle.Width(width)

	var b strings.Builder
	b.WriteString(headerStyle.Render("Detail"))
	b.WriteByte('\n')

	if d.kind == detailNone {
		b.WriteString(dimStyle.Render("  Select an item to view details"))
		b.WriteByte('\n')
	} else {
		for _, line := range strings.Split(d.content, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}

	return style.Height(height).Render(b.String())
}

func taskDetail(t taskItem, projectRoot string) detailState {
	content := fetchTaskDetail(projectRoot, t.ID)
	return detailState{kind: detailTask, content: content}
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
