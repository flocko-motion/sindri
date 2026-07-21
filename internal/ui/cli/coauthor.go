// package: ui/cli / coauthor
// type:    command (host CLI)
// job:     wires `sindri coauthor` — the one-step pairing entry: ensure a hub is
//          running (start a detached one if not), reuse or create the single
//          coauthor agent, launch it if down, and attach to its live session.
// limits:  composes existing hub/client/pod operations; no new hub behaviour. The
//          hub it starts is a normal background `sindri hub start`, so `sindri tui` in
//          another terminal works alongside it.
package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/client"
	"github.com/flo-at/sindri/internal/ui/attach"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// NewCoauthorCmd builds the `coauthor` command (a pair-programming agent).
func NewCoauthorCmd() *cobra.Command {
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
			if err := ensureHubRunning(); err != nil {
				return err
			}
			cl, err := dialHub(root)
			if err != nil {
				return err
			}
			defer cl.Close()

			proj := hub.RepoTag(root)
			name, err := ensureCoauthor(cl, proj)
			if err != nil {
				return err
			}
			if err := ensureCoauthorAlive(cl, proj, name); err != nil {
				return err
			}
			cname := hub.Container(root, name)
			fmt.Fprintf(os.Stderr, "attaching to %s — detach with your tmux prefix then d\n", name)
			// Report the coauthor to herdr for the pairing session, same as every other
			// attach path — a coauthor is an agent too. No-op outside a herdr pane.
			defer attach.ReportToHerdr(cname, name)()
			return container.ExecInteractive(cname, append([]string{"tmux"}, tmux.Attach(name, false)...)...)
		},
	}
}

// ensureCoauthor returns this project's coauthor agent's name, creating one (auto
// dwarf-named) when there isn't one yet. State lists agents across every project,
// so it must match on proj (the current repo's tag) — otherwise it would adopt a
// coauthor from another repo and then try to attach to a container that, under
// this repo's name scheme, doesn't exist.
func ensureCoauthor(cl *client.HTTP, proj string) (string, error) {
	st, err := cl.State()
	if err != nil {
		return "", err
	}
	for _, a := range st.Agents {
		if a.Role == "coauthor" && a.Project == proj {
			return a.Name, nil
		}
	}
	fmt.Fprintln(os.Stderr, "no coauthor yet — creating one…")
	name, err := cl.NewAgent("", "coauthor", "")
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
func ensureCoauthorAlive(cl *client.HTTP, proj, name string) error {
	st, err := cl.State()
	if err != nil {
		return err
	}
	if statusOf(st, proj, name) == "down" {
		fmt.Fprintf(os.Stderr, "launching agent '%s' (first run builds the agent image — may take a few minutes)…\n", name)
		if err := cl.Launch(name, false, false, os.Stderr); err != nil {
			return err
		}
	}
	for i := 0; i < 600; i++ { // ~60s after launch for the session to come up
		st, err := cl.State()
		if err != nil {
			return err
		}
		switch statusOf(st, proj, name) {
		case "", "down", "launching", "stopping":
			time.Sleep(100 * time.Millisecond)
		default: // a running phase (collab/idle/…): the session is alive
			return nil
		}
	}
	return fmt.Errorf("%s did not become ready in time", name)
}

// statusOf returns the named agent's status word from a board snapshot, or "".
// Matches on project too: agent names are unique per project, not globally.
func statusOf(st hub.BoardState, proj, name string) string {
	for _, a := range st.Agents {
		if a.Project == proj && a.Name == name {
			return a.Status
		}
	}
	return ""
}
