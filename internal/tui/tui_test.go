package tui

import (
	"strings"
	"testing"
)

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
