package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// A terminal resize must force a full clear+repaint: the alt-screen buffer keeps
// stale cells from the old (larger) frame otherwise, so a shrink renders over
// leftovers — the "ugly resize". Update must answer WindowSizeMsg with ClearScreen.
func TestResizeClearsScreen(t *testing.T) {
	var tm tea.Model = newModel(nil, nil, "")
	tm, cmd := tm.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd == nil {
		t.Fatal("resize produced no command — expected a clear+repaint")
	}
	// tea.ClearScreen's message type is unexported; compare by type against a
	// reference invocation rather than swallowing any non-nil command.
	if got, want := fmt.Sprintf("%T", cmd()), fmt.Sprintf("%T", tea.ClearScreen()); got != want {
		t.Fatalf("resize command = %s, want ClearScreen (%s)", got, want)
	}
	// The new size is applied (so the next frame is sized correctly).
	if m := tm.(model); m.w != 100 || m.h != 40 {
		t.Fatalf("resize did not apply size: got %dx%d", m.w, m.h)
	}
}

// The TUI is a hub client and must refuse to start without a running hub.
func TestRunRequiresHub(t *testing.T) {
	t.Setenv("SINDRI_HOME", t.TempDir()) // isolate: check an empty runtime dir (no hub)
	err := Run(t.TempDir())
	if err == nil {
		t.Fatal("expected an error when no hub is running")
	}
	if !strings.Contains(err.Error(), "no hub running") {
		t.Fatalf("unexpected error: %v", err)
	}
}
