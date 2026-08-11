// package: ui/cli / attach
// type:    command (host CLI)
// job:     `sindri agent attach` — hand the caller's terminal to an agent's live
// tmux session. Multi-repo like the TUI: resolves the agent from the
// global roster and narrates the dial-in (cross-repo, other clients,
// read-only) so a session never silently swallows keystrokes.
// limits:  roster + container name come from the hub; only the terminal handover
// is local (adapter/pod), since it can't travel over the socket.
package cli

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/ui/attach"
	"github.com/spf13/cobra"
)

func agentAttachCmd() *cobra.Command {
	var ro, anyway bool
	c := &cobra.Command{
		Use: "attach <name>", Short: "Attach to an agent's live tmux session (out-of-band)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			root, _ := repoRoot() // "" outside any repo — then there's no cwd context to cross
			b, err := open(root)
			if err != nil {
				return err
			}
			st, err := b.State()
			b.Close() // attach is a local pod op from here — done with the hub
			if err != nil {
				return err
			}
			a := agentByName(st.Agents, name)
			if a == nil {
				return fmt.Errorf("no such agent %q", name)
			}
			if !warnCrossRepo(a, root, projectRoot(st.Projects, a.Project)) {
				return fmt.Errorf("not attached")
			}
			cname := a.Container
			if cname == "" && root != "" { // older hub without the field — current-repo scope
				cname = container.AgentContainer(root, name)
			}
			if cname == "" {
				return fmt.Errorf("can't resolve %q's container — restart the hub to pick up this build", name)
			}
			// Trust the board's status (the same source `info`/`list` use) so the three
			// commands never contradict each other — unless the caller overrides it, since that
			// status is the last sweep's and a loaded host makes a live agent read down.
			if !anyway {
				const hint = " (--anyway attaches regardless)"
				switch {
				case a.Status == "down":
					return fmt.Errorf("agent %q is not running (status: down)%s", name, hint)
				case a.Status == api.StatusUnknown:
					return fmt.Errorf("agent %q has not been observed yet — try again in a moment%s", name, hint)
				case api.AgentNotUp(a.Status):
					return fmt.Errorf("agent %q is %s — try again in a moment%s", name, a.Status, hint)
				}
			}
			reportAttach(name, ro, a.Clients)
			// List this agent in herdr's sidebar (by name, with a live state) for the
			// duration of the dial-in, releasing on detach. No-op outside a herdr pane.
			defer attach.ReportToHerdr(cname, name)()
			return container.ExecInteractive(cname, append([]string{"tmux"}, tmux.Attach(name, ro)...)...)
		},
	}
	c.Flags().BoolVar(&ro, "read-only", false, "observe without typing")
	c.Flags().BoolVar(&anyway, "anyway", false, "attach even if the board says the agent isn't up (the status can be stale)")
	return c
}

// reportAttach narrates, before the terminal is handed to tmux, whether other
// clients are attached (a read-write attach detaches them so this dial-in gets sole
// control) and whether typing is disabled. Transparency beats a session that
// silently ignores your keystrokes. others is the current attached-client count
// from the board, so it's correct even for a cross-repo agent.
func reportAttach(name string, readOnly bool, others int) {
	if readOnly {
		fmt.Fprintf(os.Stderr, "read-only: you can watch %q but not type — re-run without --read-only to drive.\n", name)
	}
	if others == 0 {
		return
	}
	if readOnly {
		fmt.Fprintf(os.Stderr, "note: %d other client(s) already attached to %q — you'll share their view.\n", others, name)
	} else {
		fmt.Fprintf(os.Stderr, "note: %d other client(s) attached to %q — detaching them so you get sole control.\n", others, name)
	}
}
