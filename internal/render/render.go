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
		// The 📄 glyph lives in the type column (see typePrefix); the status
		// column carries only the textual state so spec rows and task rows
		// line up cleanly.
		if done, total, _ := iss.SpecProgress(); total > 0 {
			return dim.Render(fmt.Sprintf("spec %d/%d", done, total))
		}
		return dim.Render("spec")
	}
	if iss.SpecOrphan() && !iss.IsClosed() {
		// Task still open but its spec is archived or gone — surface the drift
		// instead of pretending it's a regular row. Same red-bold style as
		// the orphan-worker warning so the eye treats both as "needs fixing".
		// Closed tasks are the steady state; suppressing the warning there
		// keeps the closed history quiet.
		return orphaned.Render("⚠ spec archived")
	}
	if iss.Worker != "" {
		return Worker(iss.Worker)
	}
	if iss.Task.Status == "in_progress" {
		return Orphaned()
	}
	return TaskStatus(iss.Task.Status)
}

// TypeColumn returns the leftmost-column content for an Issue, shared by every
// interface that lays out the work list. The glyph follows the row's
// dominant identity: any issue paired with an *active* openspec change —
// spec-only or task-linked — renders 📄, replacing the type glyph it would
// otherwise carry. Plain tasks render their type glyph (🪲 bug, 🔧 feature,
// …). Orphan-linked tasks (label points at an archived/missing spec) fall
// back to the type glyph so the row's kind stays identifiable; the drift
// is surfaced separately in the status column. Children are prefixed with
// depth indent + "↳ " so the column also visualises the hierarchy.
func TypeColumn(iss issue.Issue) string {
	var b strings.Builder
	if iss.Depth > 0 {
		b.WriteString(strings.Repeat("  ", iss.Depth-1))
		b.WriteString("↳ ")
	}
	switch {
	case iss.HasSpec():
		b.WriteString("📄")
	case iss.Task != nil:
		b.WriteString(TaskTypeIcon(iss.Task.Type))
	}
	return b.String()
}

// TaskTypeIcon returns the canonical 2-cell glyph for a td task type:
//   - 🪲 bug (beetle)
//   - 🔧 feature (wrench)
//   - 🔩 task (nut and bolt)
//   - 🧹 chore (broom)
//   - 📦 epic (package)
//
// Every glyph is in the Unicode emoji block proper (no variation-selector
// hints), so terminals render them at the same 2-cell width that
// `lipgloss.Width` reports — column padding stays correct everywhere.
// Unknown type returns "", which renderers pad to the same width.
func TaskTypeIcon(typ string) string {
	switch typ {
	case "bug":
		return "🪲"
	case "feature":
		return "🔧"
	case "task":
		return "🔩"
	case "chore":
		return "🧹"
	case "epic":
		return "📦"
	default:
		return ""
	}
}

// GateLabel renders one review gate as "☑ review code" or "☐ review code".
// The dash-to-space pass keeps the "review" content visible — stripping it
// would leave just the type ("code") which is too short to read on its own.
// This is the single source of truth for gate display; CLI and TUI both
// call into it so the format stays identical.
func GateLabel(g issue.Gate) string {
	display := strings.ReplaceAll(g.Name, "-", " ")
	if g.Approved {
		return "☑ " + display
	}
	return "☐ " + display
}

// Gates renders a list of review gates, space-separated (two spaces between
// each so the eye can split them). Returns "" when there are no gates.
func Gates(gates []issue.Gate) string {
	if len(gates) == 0 {
		return ""
	}
	parts := make([]string, 0, len(gates))
	for _, g := range gates {
		parts = append(parts, GateLabel(g))
	}
	return strings.Join(parts, "  ")
}
