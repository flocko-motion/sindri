// package: paths / paths
// type:    logic (filesystem locations)
// job:     resolve sindri's machine-level directories — central state, runtime,
//          cache — honoring XDG on Linux and a SINDRI_HOME override, so the global
//          hub's state lives outside any repo.
// limits:  pure path resolution; creating and using the dirs is the caller's job.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

const appName = "sindri"

// StateDir is where the hub keeps durable state (the db, logs, per-project agent
// homes). SINDRI_HOME overrides it; otherwise it is $XDG_STATE_HOME/sindri, falling
// back to ~/.local/state/sindri (the same on Linux and macOS).
func StateDir() string {
	if h := os.Getenv("SINDRI_HOME"); h != "" {
		return h
	}
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, appName)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", appName)
}

// RuntimeDir is where the hub keeps ephemeral runtime files (the control socket and
// pid file). SINDRI_HOME overrides it; otherwise it is $XDG_RUNTIME_DIR/sindri on
// Linux (a per-user tmpfs), falling back to the state dir — macOS has no
// XDG_RUNTIME_DIR, and the control socket is host-only so its location is free.
func RuntimeDir() string {
	if h := os.Getenv("SINDRI_HOME"); h != "" {
		return h
	}
	if runtime.GOOS != "windows" {
		if x := os.Getenv("XDG_RUNTIME_DIR"); x != "" {
			return filepath.Join(x, appName)
		}
	}
	return StateDir()
}
