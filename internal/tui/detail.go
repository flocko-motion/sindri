package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
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

func taskDetail(t taskItem, prs []prItem, workers []worker.Worker, projectRoot string) detailState {
	var sections []string
	width := 80

	// Metadata section
	var meta strings.Builder
	meta.WriteString(renderField("ID", t.ID) + "\n")
	meta.WriteString(renderField("Status", statusStyle(t.Status)) + "\n")
	meta.WriteString(renderField("Type", t.Type) + "\n")
	meta.WriteString(renderField("Priority", t.Priority) + "\n")
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
	if gates := renderDetailGates(t.Labels); gates != "" {
		sections = append(sections, renderSection("Review Gates", gates, width))
	}

	// Worker section
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
		sections = append(sections, renderSection("Worker", renderField("Assigned to", assignedWorker), width))
	}

	// Associated PRs section
	var taskPRs []prItem
	var prIDs []string
	for _, pr := range prs {
		if extractTaskIDFromTitle(pr.Title) == t.ID {
			taskPRs = append(taskPRs, pr)
			prIDs = append(prIDs, pr.ID)
		}
	}
	if len(taskPRs) > 0 {
		var prContent strings.Builder
		for i, pr := range taskPRs {
			if i > 0 {
				prContent.WriteByte('\n')
			}
			prContent.WriteString(renderField("PR", pr.ID) + "\n")
			prContent.WriteString(renderField("Status", statusStyle(pr.Status)) + "\n")
			prContent.WriteString(renderField("Branch", pr.Branch+" → "+pr.Base))
		}
		sections = append(sections, renderSection("Pull Requests", prContent.String(), width))
	}

	// Comments section
	comments := fetchTaskComments(projectRoot, t.ID)
	if comments != "" && comments != "No comments" {
		sections = append(sections, renderSection("Comments", comments, width))
	}

	return detailState{
		kind:    detailTask,
		title:   t.ID + ": " + t.Title,
		content: strings.Join(sections, "\n\n"),
		taskID:  t.ID,
		prIDs:   prIDs,
	}
}

func renderDetailGates(labels []string) string {
	approved := make(map[string]bool)
	var required []string
	for _, l := range labels {
		if strings.HasPrefix(l, "require-review-") {
			required = append(required, strings.TrimPrefix(l, "require-"))
		}
		if strings.HasPrefix(l, "approved-review-") {
			approved[strings.TrimPrefix(l, "approved-")] = true
		}
	}
	if len(required) == 0 {
		return ""
	}
	var lines []string
	for _, r := range required {
		display := strings.ReplaceAll(r, "-", " ")
		if approved[r] {
			lines = append(lines, "  ☑ "+display)
		} else {
			lines = append(lines, "  ☐ "+display)
		}
	}
	return strings.Join(lines, "\n")
}

func prDetail(pr prItem) detailState {
	var sections []string
	width := 80

	var meta strings.Builder
	meta.WriteString(renderField("PR", pr.ID) + "\n")
	meta.WriteString(renderField("Title", pr.Title) + "\n")
	meta.WriteString(renderField("Branch", pr.Branch+" → "+pr.Base) + "\n")
	meta.WriteString(renderField("Status", statusStyle(pr.Status)))
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
	meta.WriteString(renderField("Status", statusStyle(wk.Status)))
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
