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
			cl, err := dialHub(root)
			if err != nil {
				return err
			}
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

// ensureHubRunning starts a detached background hub for root when none is running.
// A hub that's already up is left as-is (dialHub reconciles its version).
func ensureHubRunning(root string) error {
	if hub.IsRunning(root) {
		return nil
	}
	fmt.Fprintln(os.Stderr, "no hub running…")
	return startHub(root)
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
