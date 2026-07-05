package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateDir(t *testing.T) {
	t.Setenv("SINDRI_HOME", "")
	t.Setenv("XDG_STATE_HOME", "/xdg/state")
	if got, want := StateDir(), "/xdg/state/sindri"; got != want {
		t.Errorf("StateDir with XDG_STATE_HOME = %q, want %q", got, want)
	}

	t.Setenv("XDG_STATE_HOME", "")
	home, _ := os.UserHomeDir()
	if got, want := StateDir(), filepath.Join(home, ".local", "state", "sindri"); got != want {
		t.Errorf("StateDir fallback = %q, want %q", got, want)
	}

	t.Setenv("SINDRI_HOME", "/custom/sindri")
	if got, want := StateDir(), "/custom/sindri"; got != want {
		t.Errorf("StateDir with SINDRI_HOME = %q, want %q", got, want)
	}
}

func TestRuntimeDir(t *testing.T) {
	t.Setenv("SINDRI_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if got, want := RuntimeDir(), "/run/user/1000/sindri"; got != want {
		t.Errorf("RuntimeDir with XDG_RUNTIME_DIR = %q, want %q", got, want)
	}

	// No XDG_RUNTIME_DIR → falls back to the state dir.
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_STATE_HOME", "/xdg/state")
	if got, want := RuntimeDir(), "/xdg/state/sindri"; got != want {
		t.Errorf("RuntimeDir fallback = %q, want %q", got, want)
	}

	// SINDRI_HOME overrides everything.
	t.Setenv("SINDRI_HOME", "/custom/sindri")
	if got, want := RuntimeDir(), "/custom/sindri"; got != want {
		t.Errorf("RuntimeDir with SINDRI_HOME = %q, want %q", got, want)
	}
}
