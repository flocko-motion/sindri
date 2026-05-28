package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestPRStatusStyle(t *testing.T) {
	// Force a color profile; lipgloss strips ANSI in a non-TTY test run, which
	// would make every style render as identical plain text.
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })

	// An open PR is active/ready — green, not the orange used for open tasks.
	if got, want := prStatusStyle("open", false), prOpenStyle.Render("open"); got != want {
		t.Errorf("open PR: got %q want %q", got, want)
	}
	if got, want := prStatusStyle("approved", false), prApprovedStyle.Render("approved"); got != want {
		t.Errorf("approved PR: got %q want %q", got, want)
	}
	if got, want := prStatusStyle("merged", false), dimStyle.Render("merged"); got != want {
		t.Errorf("merged PR: got %q want %q", got, want)
	}

	// A rejected PR is red while the parent task is still active...
	if got, want := prStatusStyle("rejected", false), prRejectedStyle.Render("rejected"); got != want {
		t.Errorf("rejected PR under active task: got %q want %q", got, want)
	}
	// ...but once the task is closed the reject history is noise, so it dims.
	if got, want := prStatusStyle("rejected", true), dimStyle.Render("rejected"); got != want {
		t.Errorf("rejected PR under closed task: got %q want %q", got, want)
	}
	if prStatusStyle("rejected", true) == prRejectedStyle.Render("rejected") {
		t.Error("rejected PR under a closed task must not be rendered red")
	}
}
