// package: main (sindri) / hubclient
// type:    logic (CLI-side hub connection)
// job:     connect CLI commands to the repo's hub through one chokepoint, dialHub,
//          that first reconciles versions — if the running hub is a different build
//          than this CLI, offer to restart it. Also starts/restarts the hub.
// limits:  transport is internal/client; the pid/version stamp is internal/hub.
package main

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
	"golang.org/x/term"
)

// dialHub reconciles the running hub's version with this CLI (offering a restart on
// a mismatch), then returns a client for the repo's hub. Every command that talks
// to the hub should dial through here rather than client.Dial, so the check runs
// everywhere — coauthor, tui, and the subcommands.
func dialHub(root string) (*client.HTTP, error) {
	if err := reconcileHubVersion(root); err != nil {
		return nil, err
	}
	return client.Dial(root), nil
}

// reconcileHubVersion warns when the hub running for root was built from a
// different sindri version than this CLI and, on an interactive terminal, offers to
// restart it. It's a no-op when no hub is running or the versions already match.
// Any restart happens here, so the caller then talks to a current hub.
func reconcileHubVersion(root string) error {
	if !hub.IsRunning(root) {
		return nil // nothing to reconcile
	}
	pid, hubVer, ok := hub.ReadPID(root)
	if ok && hubVer == version {
		return nil // hub matches this CLI
	}

	if ok {
		fmt.Fprintf(os.Stderr, "warning: the hub for this repo is sindri %s but this CLI is %s.\n", hubVer, version)
	} else {
		fmt.Fprintln(os.Stderr, "warning: the hub for this repo predates version tracking, so it may be running stale code.")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "  restart it to pick up this build (stop the running `sindri hub` and re-run).")
		return nil
	}
	if !promptYesNo("restart the hub now?") {
		fmt.Fprintln(os.Stderr, "  continuing against the running hub.")
		return nil
	}
	if !ok {
		// No pid recorded (a pre-stamp hub) — we can't target it to kill it.
		return fmt.Errorf("can't restart automatically: this hub predates version tracking and recorded no pid — stop the running `sindri hub` for %s manually, then re-run", root)
	}
	return restartHub(root, pid)
}

// restartHub stops the hub with the given pid, waits for its socket to go down,
// and starts a fresh one from this binary.
func restartHub(root string, pid int) error {
	fmt.Fprintf(os.Stderr, "stopping hub (pid %d)…\n", pid)
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Signal(syscall.SIGTERM)
	}
	for i := 0; i < 50; i++ { // ~5s for it to release the socket
		if !hub.IsRunning(root) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if hub.IsRunning(root) {
		return fmt.Errorf("hub (pid %d) did not shut down — stop it and re-run", pid)
	}
	return startHub(root)
}

// startHub launches a detached background `sindri hub` for root and waits until its
// control socket answers. The hub outlives this command (own session via Setsid),
// so agents — and `sindri tui` in another terminal — keep working after we exit.
// Its output goes to .sindri/hub.log.
func startHub(root string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate the sindri binary: %w", err)
	}
	fmt.Fprintln(os.Stderr, "starting the hub in the background…")
	if err := os.MkdirAll(filepath.Join(root, ".sindri"), 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(root, ".sindri", "hub.log")
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open hub log: %w", err)
	}
	defer logf.Close()
	c := exec.Command(self, "hub")
	c.Dir = root
	c.Stdout, c.Stderr = logf, logf
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach: own session, survives us
	if err := c.Start(); err != nil {
		return fmt.Errorf("start hub: %w", err)
	}
	_ = c.Process.Release()
	for i := 0; i < 100; i++ { // ~10s for the socket to come up
		if hub.IsRunning(root) {
			fmt.Fprintf(os.Stderr, "hub up (log: %s)\n", logPath)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("hub did not come up within 10s — see %s", logPath)
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
