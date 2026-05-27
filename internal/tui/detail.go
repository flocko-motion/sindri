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
	title   string
	content string
}

func taskDetail(t taskItem, prs []prItem, workers []worker.Worker, projectRoot string) detailState {
	var b strings.Builder

	b.WriteString(fetchTaskDetail(projectRoot, t.ID))

	comments := fetchTaskComments(projectRoot, t.ID)
	if comments != "" && comments != "No comments" {
		b.WriteString("\n\n── Comments ─────────────────────────\n")
		b.WriteString(comments)
	}

	var taskPRs []prItem
	for _, pr := range prs {
		if extractTaskIDFromTitle(pr.Title) == t.ID {
			taskPRs = append(taskPRs, pr)
		}
	}
	if len(taskPRs) > 0 {
		b.WriteString("\n\n── Associated PRs ───────────────────\n")
		for _, pr := range taskPRs {
			fmt.Fprintf(&b, "%s  %s  %s → %s\n", pr.ID, statusStyle(pr.Status), pr.Branch, pr.Base)
		}
	}

	var assignedWorker string
	for _, wk := range workers {
		if wk.Task != "" {
			parts := strings.Fields(wk.Task)
			if len(parts) > 0 && parts[0] == t.ID {
				assignedWorker = wk.Name
				break
			}
		}
	}
	if assignedWorker != "" {
		b.WriteString("\n── Worker ───────────────────────────\n")
		fmt.Fprintf(&b, "Assigned to: %s\n", assignedWorker)
	}

	return detailState{kind: detailTask, title: t.ID + ": " + t.Title, content: b.String()}
}

func prDetail(pr prItem) detailState {
	var b strings.Builder
	fmt.Fprintf(&b, "PR:     %s\n", pr.ID)
	fmt.Fprintf(&b, "Title:  %s\n", pr.Title)
	fmt.Fprintf(&b, "Branch: %s → %s\n", pr.Branch, pr.Base)
	fmt.Fprintf(&b, "Status: %s\n", statusStyle(pr.Status))
	return detailState{kind: detailPR, title: pr.ID, content: b.String()}
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
	return detailState{kind: detailWorker, title: wk.Name, content: b.String()}
}
