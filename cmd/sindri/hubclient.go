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
	_, hubVer, ok := hub.ReadPID(root)
	if ok && hubVer == version {
		return nil // hub matches this CLI
	}

	if ok {
		fmt.Fprintf(os.Stderr, "warning: the hub for this repo is sindri %s but this CLI is %s.\n", hubVer, version)
	} else {
		fmt.Fprintln(os.Stderr, "warning: the hub for this repo predates version tracking, so it may be running stale code.")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "  restart it to pick up this build: `sindri hub stop` then `sindri hub start --bg`.")
		return nil
	}
	if !promptYesNo("restart the hub now?") {
		fmt.Fprintln(os.Stderr, "  continuing against the running hub.")
		return nil
	}
	pid, havePID := hub.HubPID(root) // may find a legacy hub via its socket, not just the pid file
	if !havePID {
		return fmt.Errorf("couldn't find the running hub's pid — stop it with `sindri hub stop` (or find its pid via `sindri hub list`) for %s, then re-run", root)
	}
	return restartHub(root, pid)
}

// stopHub sends SIGTERM to the hub with the given pid and waits for its control
// socket to go down.
func stopHub(root string, pid int) error {
	fmt.Fprintf(os.Stderr, "stopping hub (pid %d)…\n", pid)
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Signal(syscall.SIGTERM)
	}
	for i := 0; i < 50; i++ { // ~5s for it to release the socket
		if !hub.IsRunning(root) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("hub (pid %d) did not shut down — stop it and re-run", pid)
}

// restartHub stops the hub with the given pid and starts a fresh detached one.
func restartHub(root string, pid int) error {
	if err := stopHub(root, pid); err != nil {
		return err
	}
	return startHub(root)
}

// ensureHubRunning starts a detached background hub for root when none is running,
// so commands like `coauthor` and `tui` just work without a manual `hub start`. A
// hub that's already up is left as-is (reconcileHubVersion handles a stale one).
func ensureHubRunning(root string) error {
	if hub.IsRunning(root) {
		return nil
	}
	fmt.Fprintln(os.Stderr, "no hub running…")
	return startHub(root)
}

// startHub launches a detached background `sindri hub start` for root and waits until its
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
	c := exec.Command(self, "hub", "start")
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
