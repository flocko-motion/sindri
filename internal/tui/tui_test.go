package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The TUI is a hub client and must refuse to start without a running hub (4.3).
func TestRunRequiresHub(t *testing.T) {
	err := Run(t.TempDir())
	if err == nil {
		t.Fatal("expected an error when no hub is running")
	}
	if !strings.Contains(err.Error(), "no hub running") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Submitting the new-task form with an empty title must be rejected with a
// visible error rather than silently closing and creating nothing.
func TestNewTaskFormRejectsEmptyTitle(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	b := mockBoard()

	// n opens the form, ctrl+s tries to save it with every field left blank.
	blocked := Screenshot(b, 80, 22, "n", "ctrl+s")
	if !strings.Contains(blocked, "new task") {
		t.Fatalf("empty-title submit closed the form instead of blocking:\n%s", blocked)
	}
	if !strings.Contains(blocked, "title can't be empty") {
		t.Fatalf("expected an empty-title validation error, got:\n%s", blocked)
	}

	// Typing a title clears the error and lets the form submit (it closes).
	ok := Screenshot(b, 80, 22, "n", "h", "i", "ctrl+s")
	if strings.Contains(ok, "new task") {
		t.Fatalf("valid title should have submitted and closed the form:\n%s", ok)
	}
}
