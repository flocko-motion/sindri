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
