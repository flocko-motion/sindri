// package: main (sindri) / coauthor
// type:    command (host CLI)
// job:     wires `sindri coauthor` — the one-step pairing entry: ensure a hub is
//          running (start a detached one if not), reuse or create the single
//          coauthor agent, launch it if down, and attach to its live session.
// limits:  composes existing hub/client/pod operations; no new hub behaviour. The
//          hub it starts is a normal background `sindri hub`, so `sindri tui` in
//          another terminal works alongside it.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/flo-at/sindri/internal/adapter/pod"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newCoauthorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "coauthor",
		Short: "Pair with a coauthor: start the hub if needed, create/reuse one coauthor, and attach",
		Long: "One-step coauthor pairing. Ensures a hub is running (starting a background " +
			"one if not), reuses the existing coauthor agent or creates one, launches it if " +
			"it's down, then attaches you to its live session. Detach (your tmux prefix + d) " +
			"to leave it running; run `sindri coauthor` again to reattach, or `sindri tui` in " +
			"another terminal to add more agents.",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if !term.IsTerminal(int(os.Stdout.Fd())) {
				return fmt.Errorf("sindri coauthor requires an interactive terminal")
			}
			root, err := repoRoot()
			if err != nil {
				return err
			}
			if err := ensureHubRunning(root); err != nil {
				return err
			}
			cl := client.Dial(root)
			defer cl.Close()

			name, err := ensureCoauthor(cl)
			if err != nil {
				return err
			}
			if err := ensureCoauthorAlive(cl, name); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "attaching to %s — detach with your tmux prefix then d\n", name)
			return pod.ExecInteractive(hub.Container(root, name), append([]string{"tmux"}, tmux.Attach(name, false)...)...)
		},
	}
}

// ensureHubRunning starts a detached background `sindri hub` for root when none is
// running, then waits until its control socket answers. The hub outlives this
// command (own session via Setsid), so the coauthor — and `sindri tui` in another
// terminal — keep working after you detach. Its output goes to .sindri/hub.log.
func ensureHubRunning(root string) error {
	if hub.IsRunning(root) {
		return nil
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate the sindri binary: %w", err)
	}
	fmt.Fprintln(os.Stderr, "no hub running — starting one in the background…")
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

// ensureCoauthor returns the existing coauthor agent's name, creating one (auto
// dwarf-named) when there isn't one yet.
func ensureCoauthor(cl *client.HTTP) (string, error) {
	st, err := cl.State()
	if err != nil {
		return "", err
	}
	for _, a := range st.Agents {
		if a.Role == "coauthor" {
			return a.Name, nil
		}
	}
	fmt.Fprintln(os.Stderr, "no coauthor yet — creating one…")
	name, err := cl.NewAgent("", "coauthor")
	if err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "created coauthor %s\n", name)
	return name, nil
}

// ensureCoauthorAlive launches the coauthor when it's down and waits until its
// tmux session is live, so the attach lands on a running pod. The first launch
// also builds the agent image, which can take a few minutes (Launch blocks for
// it); a coauthor that's already running is reattached without relaunching.
func ensureCoauthorAlive(cl *client.HTTP, name string) error {
	st, err := cl.State()
	if err != nil {
		return err
	}
	if statusOf(st, name) == "down" {
		fmt.Fprintf(os.Stderr, "launching agent '%s' (first run builds the agent image — may take a few minutes)…\n", name)
		if err := cl.Launch(name, false); err != nil {
			return err
		}
	}
	for i := 0; i < 600; i++ { // ~60s after launch for the session to come up
		st, err := cl.State()
		if err != nil {
			return err
		}
		switch statusOf(st, name) {
		case "", "down", "launching", "stopping":
			time.Sleep(100 * time.Millisecond)
		default: // a running phase (collab/idle/…): the session is alive
			return nil
		}
	}
	return fmt.Errorf("%s did not become ready in time", name)
}

// statusOf returns the named agent's status word from a board snapshot, or "".
func statusOf(st hub.BoardState, name string) string {
	for _, a := range st.Agents {
		if a.Name == name {
			return a.Status
		}
	}
	return ""
}
