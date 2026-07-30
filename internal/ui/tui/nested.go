// package: tui / nested
// type:    ui (session guard)
// job:     mark every child the TUI hands the terminal to, and report whether this process is
// running inside one — so a second TUI started there is refused instead of fighting the first.
// limits:  the marker and the check; the refusal itself belongs to Run.
package tui

import (
	"os"
	"os/exec"
	"strconv"

	"github.com/flo-at/sindri/internal/hub/server"
)

// tuiPIDEnv marks a shell or editor the TUI suspended itself for, carrying that TUI's pid. A pid,
// not a flag: a flag outliving its TUI would refuse every later one here, with nothing to check.
const tuiPIDEnv = "SINDRI_TUI_PID"

// marked hands a child the marker, so anything started from it can tell where it is. Env is set
// explicitly (the child would otherwise inherit ours implicitly) and only ever by these callers,
// so appending to os.Environ() cannot drop a Cmd's own environment.
func marked(c *exec.Cmd) *exec.Cmd {
	if c == nil {
		return nil // editorAt returns nil when nothing is installed; keep that contract
	}
	c.Env = append(os.Environ(), tuiPIDEnv+"="+strconv.Itoa(os.Getpid()))
	return c
}

// ParentTUI reports the live TUI this process runs under, if any. Our own pid does not count — the
// TUI inherits what it exports — and ProcessAlive decides the rest, since a raw Kill(pid, 0)
// succeeds on a zombie and an unreaped TUI would reinstate the lockout the pid exists to avoid.
func ParentTUI() (int, bool) {
	pid, err := strconv.Atoi(os.Getenv(tuiPIDEnv))
	if err != nil || pid <= 0 || pid == os.Getpid() {
		return 0, false
	}
	if !server.ProcessAlive(pid) {
		return 0, false
	}
	return pid, true
}
