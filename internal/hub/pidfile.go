// package: hub / pidfile
// type:    logic (single-instance guard + version stamp)
// job:     record the running hub's pid and build version in .sindri/hub.pid, so a
//          second hub can't start for the same repo and clients can tell whether
//          the hub they'd talk to matches their own build.
// limits:  just the file; deciding what to do on a version mismatch is the CLI's
//          (-> cmd/sindri, which offers a restart).
package hub

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// pidInfo is what .sindri/hub.pid holds.
type pidInfo struct {
	PID     int    `json:"pid"`
	Version string `json:"version"`
}

func pidPath(root string) string { return filepath.Join(root, ".sindri", "hub.pid") }

// WritePID stamps the current process (pid + build version) as the hub for root.
// It refuses when a different, live hub already owns the repo, so two hubs can't
// serve one repo; a stale file (owner gone) is overwritten.
func WritePID(root, version string) error {
	if err := os.MkdirAll(filepath.Join(root, ".sindri"), 0o755); err != nil {
		return err
	}
	if pid, _, ok := ReadPID(root); ok && pid != os.Getpid() && ProcessAlive(pid) {
		return fmt.Errorf("a hub is already running for this repo (pid %d)", pid)
	}
	data, err := json.Marshal(pidInfo{PID: os.Getpid(), Version: version})
	if err != nil {
		return err
	}
	return os.WriteFile(pidPath(root), data, 0o644)
}

// ReadPID returns the recorded hub pid and build version, ok=false when the file
// is absent or unreadable (e.g. a hub predating this stamp).
func ReadPID(root string) (pid int, version string, ok bool) {
	data, err := os.ReadFile(pidPath(root))
	if err != nil {
		return 0, "", false
	}
	var p pidInfo
	if err := json.Unmarshal(data, &p); err != nil || p.PID == 0 {
		return 0, "", false
	}
	return p.PID, p.Version, true
}

// RemovePID clears the hub's pid file (best-effort, on shutdown).
func RemovePID(root string) { _ = os.Remove(pidPath(root)) }

// ProcessAlive reports whether pid is a live process. Signal 0 probes without
// actually signalling; EPERM means the process exists but isn't ours to signal.
func ProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
