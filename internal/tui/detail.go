package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/issue"
	"github.com/flo-at/sindri/internal/render"
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
	taskID  string
	prIDs   []string
}

var (
	sectionHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}).
				PaddingLeft(1)

	sectionBorder = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}).
			Padding(0, 1)

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#A49FA5", Dark: "#777777"}).
			Width(12)

	valueStyle = lipgloss.NewStyle()
)

func renderSection(title, body string, width int) string {
	header := sectionHeaderStyle.Render("── " + title + " ──")
	content := sectionBorder.Width(width - 4).Render(body)
	return header + "\n" + content
}

func renderField(label, value string) string {
	return labelStyle.Render(label+":") + " " + valueStyle.Render(value)
}

func issueDetail(iss issue.Issue, projectRoot string) detailState {
	if iss.SpecOnly() {
		return specDetail(iss)
	}
	t := iss.Task
	var sections []string
	width := 80

	// Metadata section
	var meta strings.Builder
	meta.WriteString(renderField("ID", iss.ID()) + "\n")
	meta.WriteString(renderField("Status", render.TaskStatus(t.Status)) + "\n")
	meta.WriteString(renderField("Type", t.Type) + "\n")
	meta.WriteString(renderField("Priority", t.Priority) + "\n")
	if iss.Spec != nil {
		meta.WriteString(renderField("Spec", iss.Spec.Name) + "\n")
	}
	if !t.CreatedAt.IsZero() {
		meta.WriteString(renderField("Created", t.CreatedAt.Local().Format("2006-01-02 15:04")) + "\n")
	}
	if !t.UpdatedAt.IsZero() {
		meta.WriteString(renderField("Updated", t.UpdatedAt.Local().Format("2006-01-02 15:04")))
	}
	sections = append(sections, renderSection("Metadata", meta.String(), width))

	// Description section
	desc := fetchTaskDetail(projectRoot, t.ID)
	if desc != "" {
		sections = append(sections, renderSection("Description", desc, width))
	}

	// Review gates section
	if gates := render.Gates(iss.Gates()); gates != "" {
		sections = append(sections, renderSection("Review Gates", gates, width))
	}

	// Worker section
	if iss.Worker != "" {
		sections = append(sections, renderSection("Worker", renderField("Assigned to", iss.Worker), width))
	}

	// Associated PRs section
	if len(iss.PRs) > 0 {
		var prContent strings.Builder
		for i, pr := range iss.PRs {
			if i > 0 {
				prContent.WriteByte('\n')
			}
			prContent.WriteString(renderField("PR", pr.ID) + "\n")
			prContent.WriteString(renderField("Status", render.PRStatus(pr.Status, iss.IsClosed())) + "\n")
			prContent.WriteString(renderField("Branch", pr.Branch+" → "+pr.Base))
		}
		sections = append(sections, renderSection("Pull Requests", prContent.String(), width))
	}

	// Comments section
	comments := fetchTaskComments(projectRoot, t.ID)
	if comments != "" && comments != "No comments" {
		sections = append(sections, renderSection("Comments", comments, width))
	}

	prIDs := make([]string, 0, len(iss.PRs))
	for _, pr := range iss.PRs {
		prIDs = append(prIDs, pr.ID)
	}
	return detailState{
		kind:    detailTask,
		title:   iss.ID() + ": " + t.Title,
		content: strings.Join(sections, "\n\n"),
		taskID:  iss.ID(),
		prIDs:   prIDs,
	}
}

func specDetail(iss issue.Issue) detailState {
	width := 80
	var meta strings.Builder
	meta.WriteString(renderField("ID", iss.ID()) + "\n")
	meta.WriteString(renderField("Spec", iss.Spec.Name) + "\n")
	if done, total, _ := iss.SpecProgress(); total > 0 {
		meta.WriteString(renderField("Tasks", fmt.Sprintf("%d/%d", done, total)))
	} else {
		meta.WriteString(renderField("Tasks", "none yet"))
	}
	body := renderSection("Spec (no task yet)", meta.String(), width)
	return detailState{
		kind:    detailTask,
		title:   iss.ID() + ": " + iss.Spec.Name,
		content: body,
		taskID:  iss.ID(),
	}
}


func prDetail(pr issue.PR) detailState {
	var sections []string
	width := 80

	var meta strings.Builder
	meta.WriteString(renderField("PR", pr.ID) + "\n")
	meta.WriteString(renderField("Title", pr.Title) + "\n")
	meta.WriteString(renderField("Branch", pr.Branch+" → "+pr.Base) + "\n")
	meta.WriteString(renderField("Status", render.PRStatus(pr.Status, false)))
	sections = append(sections, renderSection("PR Details", meta.String(), width))

	return detailState{
		kind:    detailPR,
		title:   pr.ID,
		content: strings.Join(sections, "\n\n"),
		prIDs:   []string{pr.ID},
	}
}

func workerDetail(wk worker.Worker) detailState {
	var sections []string
	width := 80

	var meta strings.Builder
	meta.WriteString(renderField("Name", wk.Name) + "\n")
	meta.WriteString(renderField("Role", wk.Role) + "\n")
	meta.WriteString(renderField("Status", render.TaskStatus(wk.Status)))
	if wk.Container != "" {
		meta.WriteString("\n" + renderField("Container", wk.Container))
	}
	if wk.Path != "" {
		meta.WriteString("\n" + renderField("Path", wk.Path))
	}
	if wk.Task != "" {
		meta.WriteString("\n" + renderField("Task", wk.Task))
	}
	if wk.PR != "" {
		meta.WriteString("\n" + renderField("PR", wk.PR))
	}
	if wk.Branch != "" {
		meta.WriteString("\n" + renderField("Branch", wk.Branch))
	}
	sections = append(sections, renderSection("Worker Details", meta.String(), width))

	return detailState{
		kind:    detailWorker,
		title:   wk.Name,
		content: strings.Join(sections, "\n\n"),
	}
}
