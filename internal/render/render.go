// package: render
// type:    rendering
// job:     maps issue/PR state to display styling (status colors, gate marks),
//          shared by every interface so the same state looks the same.
// limits:  no data logic (-> issue); no interface code (-> cmd/sindri,
//          internal/tui). Depends on issue types only.
package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/issue"
)

var (
	green  = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	dimmed = lipgloss.AdaptiveColor{Light: "#A49FA5", Dark: "#777777"}
	orange = lipgloss.Color("#FFA500")
	red    = lipgloss.Color("#FF6666")

	taskRunning = lipgloss.NewStyle().Foreground(green).Bold(true)
	taskOpen    = lipgloss.NewStyle().Foreground(orange)
	taskDone    = lipgloss.NewStyle().Foreground(dimmed)
	orphaned    = lipgloss.NewStyle().Foreground(red).Bold(true)

	prOpen     = lipgloss.NewStyle().Foreground(green)
	prApproved = lipgloss.NewStyle().Foreground(green).Bold(true)
	prRejected = lipgloss.NewStyle().Foreground(red)

	dim = lipgloss.NewStyle().Foreground(dimmed)
)

// TaskStatus renders a task status string with state-appropriate color.
func TaskStatus(status string) string {
	switch status {
	case "in_progress", "running":
		return taskRunning.Render(status)
	case "open", "in_review":
		return taskOpen.Render(status)
	case "merged", "approved", "closed":
		return taskDone.Render(status)
	default:
		return dim.Render(status)
	}
}

// Worker renders the "🔨 <name>" marker for a task a worker is on.
func Worker(name string) string {
	return taskRunning.Render("🔨 " + name)
}

// Orphaned renders the warning marker for an in_progress task with no worker.
func Orphaned() string {
	return orphaned.Render("⚠ in_progress")
}

// PRStatus colors a PR status. An open PR is active/ready, so it is green (not
// orange like an open task). A rejected PR is red only while its parent task is
// still active; once the task is closed the reject history is noise, so it dims.
func PRStatus(status string, taskClosed bool) string {
	switch status {
	case "open":
		return prOpen.Render(status)
	case "approved":
		return prApproved.Render(status)
	case "rejected":
		if taskClosed {
			return dim.Render(status)
		}
		return prRejected.Render(status)
	default: // merged, closed, unknown
		return dim.Render(status)
	}
}

// IssueStatus renders the status cell for an issue, identically for CLI and
// TUI: the assigned worker, an orphan warning, a spec marker, or the plain
// task status — colored by state.
func IssueStatus(iss issue.Issue) string {
	if iss.SpecOnly() {
		if done, total, _ := iss.SpecProgress(); total > 0 {
			return dim.Render(fmt.Sprintf("📋 spec %d/%d", done, total))
		}
		return dim.Render("📋 spec")
	}
	if iss.Worker != "" {
		return Worker(iss.Worker)
	}
	if iss.Task.Status == "in_progress" {
		return Orphaned()
	}
	return TaskStatus(iss.Task.Status)
}

// TaskTypeIcon returns the canonical glyph for a td task type — 🐛 bug,
// ✨ feature, 🧹 chore, 📦 epic. Plain "task" has no glyph (empty string), so
// most rows stay visually quiet.
func TaskTypeIcon(typ string) string {
	switch typ {
	case "bug":
		return "🐛"
	case "feature":
		return "✨"
	case "chore":
		return "🧹"
	case "epic":
		return "📦"
	default:
		return ""
	}
}

// Gates renders review gates as "☑ name" / "☐ name", space-separated.
// Returns "" when there are no gates.
func Gates(gates []issue.Gate) string {
	if len(gates) == 0 {
		return ""
	}
	var parts []string
	for _, g := range gates {
		display := strings.ReplaceAll(g.Name, "-", " ")
		if g.Approved {
			parts = append(parts, "☑ "+display)
		} else {
			parts = append(parts, "☐ "+display)
		}
	}
	return strings.Join(parts, "  ")
}
