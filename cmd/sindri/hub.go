// package: main (sindri) / hub commands
// type:    command (host CLI)
// job:     the hub-era verbs — `hub` (run the persistent service), `new`
//          (register an identity), `launch` (spin a pod), `tell` (inject a
//          message), `attach` (dial into the live session), `agents` (list).
// limits:  no logic — every verb is a thin call into a backend (in-process hub
//          when none is running, the socket client otherwise).
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/adapter/pod"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/spf13/cobra"
)

// backend is the hub operation set; satisfied by both *hub.Hub (in-process,
// ephemeral) and *client.HTTP (a running hub over its socket).
type backend interface {
	NewAgent(name, role string) error
	Launch(name string) error
	Tell(name, msg, source string) error
	State() ([]hub.AgentState, error)
	Close() error
}

// repoRoot resolves the git root from the current directory.
func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return git.Root(wd)
}

// open returns a backend: the running hub over its socket if one is up, else an
// ephemeral in-process hub (serve this one call, exit).
func open(root string) (backend, error) {
	if hub.IsRunning(root) {
		return client.Dial(root), nil
	}
	return hub.New(root)
}

func newHubCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hub",
		Short: "Run the per-repo hub service (foreground)",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			if hub.IsRunning(root) {
				return fmt.Errorf("a hub is already running for this repo (%s)", hub.SocketPath(root))
			}
			h, err := hub.New(root)
			if err != nil {
				return err
			}
			defer h.Close()
			fmt.Fprintf(os.Stderr, "sindri hub listening at %s\n", h.SocketPath())
			return h.Serve()
		},
	}
}

func newNewCmd() *cobra.Command {
	var role string
	c := &cobra.Command{
		Use:   "new <name>",
		Short: "Register an agent identity (no pod) — identity precedes runtime",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.NewAgent(args[0], role); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "registered %s (%s) — launch with 'sindri launch %s'\n", args[0], role, args[0])
				return nil
			})
		},
	}
	c.Flags().StringVar(&role, "role", "worker", "agent role: worker|reviewer")
	return c
}

func newLaunchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "launch <name>",
		Short: "Spin a pod that assumes an existing agent's identity",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			// A launched agent needs its socket served for as long as it runs, so a
			// persistent hub must be up — an ephemeral in-process hub would take the
			// listener down on exit.
			if !hub.IsRunning(root) {
				return fmt.Errorf("no hub running — start one first: 'sindri hub &' (agents need a persistent hub)")
			}
			c := client.Dial(root)
			defer c.Close()
			if err := c.Launch(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "launched %s\n", args[0])
			return nil
		},
	}
}

func newTellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tell <name> <message...>",
		Short: "Send a message into an agent's session (stamped [user])",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			msg := strings.Join(args[1:], " ")
			return withBackend(func(b backend) error {
				if err := b.Tell(args[0], msg, "user"); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "delivered to %s\n", args[0])
				return nil
			})
		},
	}
}

func newAttachCmd() *cobra.Command {
	var ro bool
	c := &cobra.Command{
		Use:   "attach <name>",
		Short: "Attach to an agent's live tmux session (out-of-band, bypasses the hub)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			c := hub.Container(name)
			if !pod.Running(c) {
				return fmt.Errorf("agent %q is not running", name)
			}
			return pod.ExecInteractive(c, append([]string{"tmux"}, tmux.Attach(name, ro)...)...)
		},
	}
	c.Flags().BoolVar(&ro, "read-only", false, "observe without typing")
	return c
}

func newAgentsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "agents",
		Short: "List registered agents and their live status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				st, err := b.State()
				if err != nil {
					return err
				}
				if len(st) == 0 {
					fmt.Fprintln(os.Stderr, "no agents — register one with 'sindri new <name>'")
					return nil
				}
				for _, a := range st {
					status := "stopped"
					if a.Running {
						status = "running"
					}
					fmt.Printf("%-12s %-8s %s\n", a.Name, a.Role, status)
				}
				return nil
			})
		},
	}
}

// withBackend opens a backend for the repo, runs fn, and closes it.
func withBackend(fn func(backend) error) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	b, err := open(root)
	if err != nil {
		return err
	}
	defer b.Close()
	return fn(b)
}
