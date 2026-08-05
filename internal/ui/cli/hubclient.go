// package: ui/cli / hubclient
// type:    logic (CLI-side hub connection)
// job:     connect CLI commands to the single global hub through one chokepoint,
// dialHub, which reconciles versions (offering a restart on a mismatch).
// Also starts/restarts the background hub. One hub per machine; commands
// tag their repo via the client's X-Sindri-Project header.
// limits:  transport is internal/client; the pid/version stamp is internal/hub.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/tools/paths"
	"golang.org/x/term"
)

// dialHub reconciles the hub's version, then returns a client tagged with root. Dial through here.
func dialHub(root string) (*client.HTTP, error) {
	if err := reconcileHubVersion(); err != nil {
		return nil, err
	}
	return client.Dial(root), nil
}

// reconcileHubVersion warns on a hub/CLI version mismatch and, on a terminal, offers a restart.
func reconcileHubVersion() error {
	if !hub.IsRunning() {
		return nil // nothing to reconcile
	}
	_, hubVer, ok := hub.ReadPID()
	if ok && hubVer == version {
		return nil // hub matches this CLI
	}

	if ok {
		fmt.Fprintf(os.Stderr, "warning: the running hub is sindri %s but this CLI is %s.\n", hubVer, version)
	} else {
		fmt.Fprintln(os.Stderr, "warning: the running hub predates version tracking, so it may be stale code.")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "  restart it to pick up this build: `sindri hub stop` then `sindri hub start --bg`.")
		return nil
	}
	if !promptYesNo("restart the hub now?") {
		fmt.Fprintln(os.Stderr, "  continuing against the running hub.")
		return nil
	}
	pid, havePID := hub.HubPID() // pid file, or the socket's owner via lsof
	if !havePID {
		return fmt.Errorf("couldn't find the running hub's pid — stop it with `sindri hub stop`, then re-run")
	}
	return restartHub(pid)
}

// stopHub SIGTERMs the hub with the given pid and waits for its control socket to go down.
func stopHub(pid int) error {
	fmt.Fprintf(os.Stderr, "stopping hub (pid %d)…\n", pid)
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Signal(syscall.SIGTERM)
	}
	for i := 0; i < 50; i++ { // ~5s for it to release the socket
		if !hub.IsRunning() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("hub (pid %d) did not shut down — stop it and re-run", pid)
}

// restartHub stops the hub with the given pid and starts a fresh detached one.
func restartHub(pid int) error {
	if err := stopHub(pid); err != nil {
		return err
	}
	return startHub()
}

// startupBudget is how long a starting hub gets to answer its socket, polled every startupPoll.
// Generous on purpose: 10s read as a failed start on any busy machine, and a dead hub fails fast
// anyway via the loop's liveness check.
const (
	startupBudget = 45 * time.Second
	startupPoll   = 100 * time.Millisecond
	// startupAttempts derives from the two above so the budget can't drift from the loop enforcing it.
	startupAttempts = int(startupBudget / startupPoll)
)

// ensureHubRunning starts the detached hub when none answers, narrating each step so a failed start
// isn't a mystery. A running hub is left as-is (reconcileHubVersion handles a stale one).
func ensureHubRunning() error {
	fmt.Fprint(os.Stderr, "looking for a running hub… ")
	if hub.IsRunning() {
		fmt.Fprintln(os.Stderr, "found one, answering its socket.")
		return nil
	}
	fmt.Fprintln(os.Stderr, "none answering.")
	// A leftover pid record is common — a hub killed without cleanup, or a zombie held by its parent.
	if pid, _, ok := hub.ReadPID(); ok {
		if hub.ProcessAlive(pid) {
			fmt.Fprintf(os.Stderr, "a hub (pid %d) is recorded and alive but not answering — it may be hung; will try to start a fresh one.\n", pid)
		} else {
			fmt.Fprintf(os.Stderr, "found a stale hub record (pid %d, no longer serving); replacing it.\n", pid)
		}
	}
	return startHub()
}

// startHub launches a detached `sindri hub start` (own session, so agents and a `sindri tui` elsewhere
// survive this command exiting) and waits for its socket, logging to the state dir's hub.log.
func startHub() error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate the sindri binary: %w", err)
	}
	fmt.Fprintln(os.Stderr, "starting a new hub in the background…")
	if err := os.MkdirAll(paths.StateDir(), 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(paths.StateDir(), "hub.log")
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open hub log: %w", err)
	}
	defer logf.Close()
	c := exec.Command(self, "hub", "start")
	c.Stdout, c.Stderr = logf, logf
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach: own session, survives us
	if err := c.Start(); err != nil {
		return fmt.Errorf("start hub: %w", err)
	}
	pid := c.Process.Pid
	_ = c.Process.Release()
	fmt.Fprint(os.Stderr, "waiting for it to answer the health check…")
	for i := 0; i < startupAttempts; i++ {
		if hub.IsRunning() {
			fmt.Fprintf(os.Stderr, " up (pid %d, log: %s)\n", pid, logPath)
			return nil
		}
		// Fast-fail: an exited process will never answer, so report why instead of burning the budget.
		if !hub.ProcessAlive(pid) {
			fmt.Fprintln(os.Stderr, " it exited.")
			return fmt.Errorf("hub failed to start: %s (see %s)", lastLogLine(logPath), logPath)
		}
		time.Sleep(startupPoll)
	}
	fmt.Fprintln(os.Stderr, " timed out.")
	return fmt.Errorf("hub did not come up within %s: %s (see %s)", startupBudget, lastLogLine(logPath), logPath)
}

// lastLogLine returns the hub log's last non-empty line, so a failed start surfaces the hub's own error.
func lastLogLine(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "no log output"
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return "no log output"
}

// promptYesNo asks q on stderr and reads a yes/no answer from stdin (default yes).
func promptYesNo(q string) bool {
	fmt.Fprintf(os.Stderr, "%s [Y/n] ", q)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes":
		return true
	default:
		return false
	}
}
