// package: hub/server / pidfile
// type:    logic (single-instance guard + version stamp)
// job:     record the running global hub's pid and build version under the runtime
// dir, so a second hub can't start and clients can tell whether the hub
// they'd talk to matches their own build.
// limits:  just the file; deciding what to do on a version mismatch is the CLI's
// (-> cmd/sindri, which offers a restart).
package server

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/flo-at/sindri/internal/tools/paths"
)

// pidInfo is what hub.pid holds.
type pidInfo struct {
	PID     int    `json:"pid"`
	Version string `json:"version"`
}

func pidPath() string { return filepath.Join(paths.RuntimeDir(), "hub.pid") }

// WritePID stamps the current process (pid + build version) as the global hub. It
// refuses when a different, live hub already owns the runtime dir; a stale file
// (owner gone) is overwritten.
func WritePID(version string) error {
	if err := os.MkdirAll(paths.RuntimeDir(), 0o755); err != nil {
		return err
	}
	if pid, _, ok := ReadPID(); ok && pid != os.Getpid() && processAlive(pid) {
		return fmt.Errorf("a hub is already running (pid %d)", pid)
	}
	data, err := json.Marshal(pidInfo{PID: os.Getpid(), Version: version})
	if err != nil {
		return err
	}
	return os.WriteFile(pidPath(), data, 0o644)
}

// ReadPID returns the recorded hub pid and build version, ok=false when the file
// is absent or unreadable.
func ReadPID() (pid int, version string, ok bool) {
	data, err := os.ReadFile(pidPath())
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
func RemovePID() { _ = os.Remove(pidPath()) }

// processAlive reports whether pid is a live process that could still be serving —
// not merely one that occupies a slot in the process table. Signal 0 probes existence
// (EPERM means it exists but isn't ours to signal), but a zombie (a dead child not yet
// reaped by its parent) also answers signal 0 while being unable to do anything. A
// zombie hub is a dead hub, so it must not count as alive or it would wedge every
// restart; we reject it explicitly. Unexported: only WritePID's own race guard needs
// it here — internal/client carries the copy front-ends use to find/stop a hub.
func processAlive(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil && err != syscall.EPERM {
		return false
	}
	return !isZombie(pid)
}

// isZombie reports whether pid is a zombie (defunct): still in the process table,
// holding its exit status until the parent reaps it, but dead. Best-effort — if we
// can't read the state we assume it's not a zombie, so we never discard a hub that
// might be live.
func isZombie(pid int) bool {
	out, err := exec.Command("ps", "-o", "state=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(string(out)), "Z")
}
