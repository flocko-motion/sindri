// package: tui / detail
// type:    ui
// job:     renders the multi-pane detail view for an issue, PR, or worker
//          (metadata, description, gates, PRs, comments).
// limits:  read-only rendering — actions live in actions.go; data via
//          adapter/td (detail text) and the issue model.
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
	kind       detailKind
	title      string
	leftCol    string // formal data: metadata, gates, worker, PRs
	rightCol   string // free text: description, acceptance, comments — scrollable
	content    string // legacy single-column content for PR/worker details (still one column)
	taskID     string
	taskStatus string // current task status — used to pre-select the status picker
	prIDs      []string
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

func issueDetail(iss issue.Issue, projectRoot string, colWidth int) detailState {
	if colWidth < 20 {
		colWidth = 40 // safety floor for very narrow terminals / tests that didn't pass a width
	}
	if iss.SpecOnly() {
		return specDetail(iss, colWidth)
	}
	t := iss.Task

	// --- Left column: formal data the eye scans (metadata, gates, worker, PRs).
	var leftSections []string

	var meta strings.Builder
	meta.WriteString(renderField("ID", iss.ID()) + "\n")
	meta.WriteString(renderField("Status", render.TaskStatus(t.Status)) + "\n")
	meta.WriteString(renderField("Type", t.Type) + "\n")
	meta.WriteString(renderField("Priority", t.Priority))
	if iss.Spec != nil {
		meta.WriteString("\n" + renderField("Spec", iss.Spec.Name))
	}
	if !t.CreatedAt.IsZero() {
		meta.WriteString("\n" + renderField("Created", t.CreatedAt.Local().Format("2006-01-02 15:04")))
	}
	if !t.UpdatedAt.IsZero() {
		meta.WriteString("\n" + renderField("Updated", t.UpdatedAt.Local().Format("2006-01-02 15:04")))
	}
	leftSections = append(leftSections, renderSection("Metadata", meta.String(), colWidth))

	if gates := render.Gates(iss.Gates()); gates != "" {
		leftSections = append(leftSections, renderSection("Review Gates", gates, colWidth))
	}

	if iss.Worker != "" {
		leftSections = append(leftSections, renderSection("Worker", renderField("Assigned", iss.Worker), colWidth))
	}

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
		leftSections = append(leftSections, renderSection("Pull Requests", prContent.String(), colWidth))
	}

	// --- Right column: free-text body. Description and acceptance come from the
	// structured td show --json fields, NOT the textual `td show`, so the
	// metadata block doesn't get echoed back into the description and re-read
	// next to the left pane.
	var rightSections []string
	if desc := strings.TrimSpace(fetchTaskDetail(projectRoot, t.ID)); desc != "" {
		rightSections = append(rightSections, renderSection("Description", desc, colWidth))
	}
	if acc := strings.TrimSpace(fetchTaskAcceptance(projectRoot, t.ID)); acc != "" {
		rightSections = append(rightSections, renderSection("Acceptance", acc, colWidth))
	}
	if comments := strings.TrimSpace(fetchTaskComments(projectRoot, t.ID)); comments != "" && comments != "No comments" {
		rightSections = append(rightSections, renderSection("Comments", comments, colWidth))
	}

	prIDs := make([]string, 0, len(iss.PRs))
	for _, pr := range iss.PRs {
		prIDs = append(prIDs, pr.ID)
	}
	return detailState{
		kind:       detailTask,
		title:      iss.ID() + ": " + t.Title,
		leftCol:    strings.Join(leftSections, "\n\n"),
		rightCol:   strings.Join(rightSections, "\n\n"),
		taskID:     iss.ID(),
		taskStatus: t.Status,
		prIDs:      prIDs,
	}
}

func specDetail(iss issue.Issue, colWidth int) detailState {
	var meta strings.Builder
	meta.WriteString(renderField("ID", iss.ID()) + "\n")
	meta.WriteString(renderField("Spec", iss.Spec.Name))
	if done, total, _ := iss.SpecProgress(); total > 0 {
		meta.WriteString("\n" + renderField("Tasks", fmt.Sprintf("%d/%d", done, total)))
	} else {
		meta.WriteString("\n" + renderField("Tasks", "none yet"))
	}
	return detailState{
		kind:    detailTask,
		title:   iss.ID() + ": " + iss.Spec.Name,
		leftCol: renderSection("Spec (no task yet)", meta.String(), colWidth),
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
