// package: main (sindri) / agent
// type:    command (host CLI)
// job:     the `sindri agent` subcommands other than attach — list, new, delete,
//          pane, start, stop, restart, tell, info — each a thin call into the hub
//          backend. Attach is its own file (attach.go).
// limits:  no logic; every verb marshals to the hub via the backend port.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// agentPreflight warns — without blocking — when podman is unreachable. Every
// agent subcommand ultimately needs pods, so infrastructure being offline is the
// likeliest reason nothing works; say so up front (e.g. "all agents down") instead
// of leaving the user to infer it. The probe is time-bounded (see container.Healthy) so
// a wedged VM can't hang the command.
func agentPreflight(*cobra.Command, []string) {
	if ok, hint := container.Healthy(); !ok {
		fmt.Fprintf(os.Stderr, "warning: %s\n", hint)
	}
}

// agentByName finds an agent by name in the global roster, nil when absent.
func agentByName(agents []hub.AgentView, name string) *hub.AgentView {
	for i := range agents {
		if agents[i].Name == name {
			return &agents[i]
		}
	}
	return nil
}

// projectRoot maps an agent's project tag to its on-disk repo root, "" if unknown.
func projectRoot(projects []store.Project, tag string) string {
	for _, p := range projects {
		if p.Tag == tag {
			return p.Path
		}
	}
	return ""
}

// warnCrossRepo raises awareness when the target agent lives in a different repo
// than the caller's cwd — the CLI manages agents globally, like the TUI, so this
// never fails, it just makes the context switch conscious. On a terminal it asks
// to proceed (declining returns false); non-interactively it proceeds after the
// note. cwdRoot=="" (outside any repo), or an unknown/matching project, means
// there's nothing to cross. Shared by every agent subcommand, incl. attach.
func warnCrossRepo(a *hub.AgentView, cwdRoot, agentRoot string) bool {
	if cwdRoot == "" || agentRoot == "" || agentRoot == cwdRoot {
		return true
	}
	fmt.Fprintf(os.Stderr, "note: agent %q lives in project %q, not the repo you're in.\n", a.Name, a.Repo)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return true
	}
	return promptYesNo(fmt.Sprintf("act on it in %s's context?", a.Repo))
}

// withAgent runs a hub operation on the agent named `name`, resolved from the
// global roster and scoped to its own project — so any agent is manageable from
// any cwd (the CLI is global, like the TUI), not just those in the current repo.
// It warns on a cross-repo reach instead of failing with "no such agent". fn gets
// a backend scoped to the agent's project.
func withAgent(name string, fn func(b backend, a *hub.AgentView) error) error {
	root, _ := repoRoot() // "" outside any repo — then there's no cwd context to cross
	b, err := open(root)
	if err != nil {
		return err
	}
	st, err := b.State()
	if err != nil {
		b.Close()
		return err
	}
	a := agentByName(st.Agents, name)
	if a == nil {
		b.Close()
		return fmt.Errorf("no such agent %q", name)
	}
	agentRoot := projectRoot(st.Projects, a.Project)
	if !warnCrossRepo(a, root, agentRoot) {
		b.Close()
		return fmt.Errorf("cancelled")
	}
	if agentRoot != "" && agentRoot != root { // re-scope the client to the agent's project
		b.Close()
		if b, err = dialHub(agentRoot); err != nil {
			return err
		}
	}
	defer b.Close()
	return fn(b, a)
}

func agentListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List agents with their live state", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return withBackend(func(b backend) error {
				st, err := b.State()
				if err != nil {
					return err
				}
				for _, a := range st.Agents {
					line := fmt.Sprintf("%-10.10s %-12s %-8s %-10s %-14s %s", a.Repo, a.Name, a.Role, a.Status, dash(a.Task), dash(a.PR))
					if a.Clients > 0 {
						line += fmt.Sprintf("  👁%d", a.Clients)
					}
					fmt.Println(line)
				}
				for _, o := range st.Orphans {
					fmt.Printf("⚠  orphan: %s — no roster entry; remove with 'podman rm -f %s'\n", o, o)
				}
				if len(st.Agents) == 0 && len(st.Orphans) == 0 {
					fmt.Fprintln(os.Stderr, "no agents — register one with 'sindri agent new <name>'")
				}
				return nil
			})
		},
	}
}

func agentNewCmd() *cobra.Command {
	var role string
	c := &cobra.Command{
		Use: "new [name]", Short: "Register an agent identity (no pod; name optional — auto dwarf name)", Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var want string
			if len(args) == 1 {
				want = args[0]
			}
			return withBackend(func(b backend) error {
				name, err := b.NewAgent(want, role)
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "registered %s (%s) — start with 'sindri agent start %s'\n", name, role, name)
				return nil
			})
		},
	}
	c.Flags().StringVar(&role, "role", "worker", "agent role: worker|reviewer|planner|coauthor")
	return c
}

func agentDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use: "delete <name>", Aliases: []string{"rm"}, Short: "Delete an agent (pod, socket, worktree, identity)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *hub.AgentView) error {
				if err := b.DeleteAgent(a.Name); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "deleted %s\n", a.Name)
				return nil
			})
		},
	}
}

func agentPaneCmd() *cobra.Command {
	var lines int
	c := &cobra.Command{
		Use: "pane <name>", Short: "Print the agent's live tmux screen (capture-pane)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *hub.AgentView) error {
				out, err := b.AgentPane(a.Name, lines)
				if err != nil {
					return err
				}
				if out == "" {
					fmt.Fprintln(os.Stderr, "(no live screen — agent is down)")
					return nil
				}
				fmt.Print(out)
				return nil
			})
		},
	}
	c.Flags().IntVarP(&lines, "lines", "n", 40, "rows of scrollback to capture")
	return c
}

func agentStartCmd() *cobra.Command {
	var shell bool
	c := &cobra.Command{
		Use: "start <name>", Short: "Start the agent: spin a pod that assumes its identity (runs Claude)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *hub.AgentView) error {
				if err := b.Launch(a.Name, shell); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "started %s\n", a.Name)
				return nil
			})
		},
	}
	c.Flags().BoolVar(&shell, "shell", false, "run a bare shell instead of Claude (debug/demo)")
	return c
}

func agentStopCmd() *cobra.Command {
	return &cobra.Command{
		Use: "stop <name>", Short: "Tear down the agent's pod (keeps its identity)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *hub.AgentView) error {
				if err := b.StopAgent(a.Name); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "stopped %s\n", a.Name)
				return nil
			})
		},
	}
}

// agentRestartCmd stops the agent's pod and starts a fresh one — the way to pick
// up a rebuilt agent image or clear a wedged session. If the agent wasn't running,
// it's just a start (no error), so `restart` is always safe to reach for.
func agentRestartCmd() *cobra.Command {
	var shell bool
	c := &cobra.Command{
		Use: "restart <name>", Short: "Restart the agent's pod (starts it if it wasn't running)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *hub.AgentView) error {
				verb := "restarted"
				if a.Status == "down" {
					verb = "started" // wasn't running — a plain start, not a restart
				} else if err := b.StopAgent(a.Name); err != nil {
					return err
				}
				if err := b.Launch(a.Name, shell); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "%s %s\n", verb, a.Name)
				return nil
			})
		},
	}
	c.Flags().BoolVar(&shell, "shell", false, "run a bare shell instead of Claude (debug/demo)")
	return c
}

func agentTellCmd() *cobra.Command {
	return &cobra.Command{
		Use: "tell <name> <message...>", Short: "Send a message into an agent's session ([user])", Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			msg := strings.Join(args[1:], " ")
			return withAgent(args[0], func(b backend, a *hub.AgentView) error {
				if err := b.Tell(a.Name, msg, "user"); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "delivered to %s\n", a.Name)
				return nil
			})
		},
	}
}

func agentInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use: "info <name>", Short: "Show an agent's state and activity timeline", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, found *hub.AgentView) error {
				fmt.Printf("agent:     %s\nrole:      %s\nstatus:    %s\ntask:      %s\npr:        %s\nworkspace: %s\n",
					found.Name, found.Role, found.Status, dash(found.Task), dash(found.PR), dash(found.Workspace))
				if cs, err := b.Clients(found.Name); err == nil {
					fmt.Print(hub.FormatClients(cs))
				}
				evs, err := b.Log(found.Name)
				if err != nil {
					return err
				}
				fmt.Println("\nactivity:")
				for _, e := range evs {
					fmt.Printf("  %-10s %s\n", e.Type, e.Payload)
				}
				return nil
			})
		},
	}
}
