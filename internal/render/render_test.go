package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/issue"
	"github.com/muesli/termenv"
)

func TestPRStatus(t *testing.T) {
	// Force a color profile; lipgloss strips ANSI in a non-TTY test run, which
	// would make every style render as identical plain text.
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })

	// open and approved are colored (active/ready) — not dimmed like merged.
	if PRStatus("open", false) == PRStatus("merged", false) {
		t.Error("open PR must not render the same as merged (dim)")
	}
	if PRStatus("approved", false) == PRStatus("merged", false) {
		t.Error("approved PR must not render the same as merged (dim)")
	}

	// A rejected PR is red while the parent task is active, dim once it closes.
	if PRStatus("rejected", false) == PRStatus("rejected", true) {
		t.Error("rejected PR should look different under an active vs closed task")
	}
	// Once the task is closed, rejected is just dimmed.
	if PRStatus("rejected", true) != dim.Render("rejected") {
		t.Error("rejected PR under a closed task should be dimmed")
	}
}

func TestGates(t *testing.T) {
	if got := Gates(nil); got != "" {
		t.Errorf("Gates(nil) = %q want empty", got)
	}
	out := Gates([]issue.Gate{
		{Name: "code", Approved: true},
		{Name: "security", Approved: false},
	})
	if !strings.Contains(out, "☑ code") {
		t.Errorf("Gates output missing approved code gate: %q", out)
	}
	if !strings.Contains(out, "☐ security") {
		t.Errorf("Gates output missing unapproved security gate: %q", out)
	}
}
