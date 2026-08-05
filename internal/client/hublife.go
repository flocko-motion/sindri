// package: client / hublife
// type:    logic (hub discovery, client-side)
// job:     find out whether a hub is up and, if so, its pid — the checks the CLI
// makes before starting, stopping or restarting one. Reads the same pid file
// the hub writes (internal/hub/server/pidfile.go); this is the client-side
// half so front-ends need not import that hub-side package to reach it.
// limits:  discovery only; writing the pid file is the hub's own job.
package client

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/flo-at/sindri/internal/tools/paths"
)

// IsRunning reports whether a hub is listening on the control socket.
func IsRunning() bool {
	c, err := net.DialTimeout("unix", paths.HubSocket(), 300*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// pidInfo mirrors what the hub's pidfile.go writes.
type pidInfo struct {
	PID     int    `json:"pid"`
	Version string `json:"version"`
}

func pidPath() string { return filepath.Join(paths.RuntimeDir(), "hub.pid") }

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

// ProcessAlive reports whether pid is a live process that could still be serving
// — not merely one that occupies a slot in the process table. Signal 0 probes
// existence (EPERM means it exists but isn't ours to signal), but a zombie (a dead
// child not yet reaped by its parent) also answers signal 0 while being unable to
// do anything. A zombie hub is a dead hub, so it must not count as alive or it
// would wedge every restart; we reject it explicitly.
func ProcessAlive(pid int) bool {
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

// HubPID returns the pid of the running hub, preferring the pid file and falling
// back to whoever holds the control socket (via lsof). ok=false when none found.
func HubPID() (pid int, ok bool) {
	if p, _, isok := ReadPID(); isok && ProcessAlive(p) {
		return p, true
	}
	out, err := exec.Command("lsof", "-t", paths.HubSocket()).Output()
	if err != nil {
		return 0, false
	}
	for _, f := range strings.Fields(string(out)) {
		if p, err := strconv.Atoi(f); err == nil && ProcessAlive(p) {
			return p, true
		}
	}
	return 0, false
}
