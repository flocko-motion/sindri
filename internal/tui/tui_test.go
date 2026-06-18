package tui

import (
	"strings"
	"testing"
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
