// package: ui/cli / agent
// type:    command (host CLI)
// job:     the `sindri agent` subcommands other than attach — list, new, delete,
// pane, start, stop, restart, tell, info — each a thin call into the hub
// backend. Attach is its own file (attach.go).
// limits:  no logic; every verb marshals to the hub via the backend port.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/ui/theme"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// agentPreflight warns without blocking when podman is unreachable: it is the likeliest
// reason nothing works, so say so rather than let the user infer it from "all agents down".
func agentPreflight(*cobra.Command, []string) {
	if ok, hint := container.Healthy(); !ok {
		fmt.Fprintf(os.Stderr, "warning: %s\n", hint)
	}
}

// agentByName finds an agent by name in the global roster, nil when absent.
func agentByName(agents []api.AgentView, name string) *api.AgentView {
	for i := range agents {
		if agents[i].Name == name {
			return &agents[i]
		}
	}
	return nil
}

// projectRoot maps an agent's project tag to its on-disk repo root, "" if unknown.
func projectRoot(projects []api.Project, tag string) string {
	for _, p := range projects {
		if p.Tag == tag {
			return p.Path
		}
	}
	return ""
}

// warnCrossRepo makes reaching into another repo conscious without ever failing: the CLI is
// global like the TUI. A terminal is asked to confirm; non-interactive proceeds after the note.
func warnCrossRepo(a *api.AgentView, cwdRoot, agentRoot string) bool {
	if cwdRoot == "" || agentRoot == "" || agentRoot == cwdRoot {
		return true
	}
	fmt.Fprintf(os.Stderr, "note: agent %q lives in project %q, not the repo you're in.\n", a.Name, a.Repo)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return true
	}
	return promptYesNo(fmt.Sprintf("act on it in %s's context?", a.Repo))
}

// withAgent resolves name in the global roster and hands fn a backend scoped to the agent's
// own project, so any agent is manageable from any cwd instead of erroring "no such agent".
func withAgent(name string, fn func(b backend, a *api.AgentView) error) error {
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
				for _, a := range api.SortedAgents(st.Agents, st.Projects) {
					line := fmt.Sprintf("%-10.10s %-12s %-8s %-10s %4s %-14s %s", a.Repo, a.Name, a.Role, a.Status,
						theme.ContextPercent(a.ContextTokens, a.ContextWindow), dash(a.Task), dash(a.PR))
					if a.Clients > 0 {
						line += fmt.Sprintf("  👁%d", a.Clients)
					}
					fmt.Println(line)
				}
				for _, o := range st.Orphans {
					fmt.Printf("⚠  orphan: %s — no roster entry; remove with 'sindri agent delete %s'\n", o, o)
				}
				if len(st.Agents) == 0 && len(st.Orphans) == 0 {
					fmt.Fprintln(os.Stderr, "no agents — register one with 'sindri agent new <name>'")
				}
				return nil
			})
		},
	}
}

// agentStatsCmd is the view for tuning per-agent memory; down agents have no VM to sample.
func agentStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use: "stats [name]", Short: "Show each running agent's VM memory usage vs its limit", Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				report, err := b.Stats()
				if err != nil {
					return err
				}
				views := report.Agents
				if len(args) == 1 { // narrow to one agent
					var only []api.AgentStatsView
					for _, v := range views {
						if v.Name == args[0] {
							only = append(only, v)
						}
					}
					views = only
				}
				fmt.Printf("engine: %s\n\n", report.Engine)
				if len(views) == 0 {
					fmt.Fprintln(os.Stderr, "no running agents to sample")
					return nil
				}
				fmt.Printf("%-10.10s %-12s %s\n", "REPO", "AGENT", "MEMORY")
				for _, v := range views {
					if v.Err != "" { // surface the reason, don't hide it behind a blank row
						fmt.Printf("%-10.10s %-12s stats unavailable: %s\n", v.Repo, v.Name, v.Err)
						continue
					}
					fmt.Printf("%-10.10s %-12s %s\n", v.Repo, v.Name, theme.MemLine(v.MemUsageBytes, v.MemLimitBytes))
				}
				return nil
			})
		},
	}
}

func agentNewCmd() *cobra.Command {
	var role, memory string
	var noStart bool
	c := &cobra.Command{
		Use: "new [name]", Short: "Create an agent and start it (name optional — auto dwarf name)", Args: cobra.MaximumNArgs(1),
		Long: "Register an agent identity and start its container, which is what you almost always want —\n" +
			"the same thing the TUI's 'new' does.\n\n" +
			"--no-start registers the identity alone, for pre-declaring an agent you will start later.\n" +
			"An agent exists independently of any container, so this is a supported state, not a failure.",
		RunE: func(_ *cobra.Command, args []string) error {
			var want string
			if len(args) == 1 {
				want = args[0]
			}
			return withBackend(func(b backend) error {
				name, err := b.NewAgent(want, role, memory)
				if err != nil {
					return err
				}
				if noStart {
					fmt.Fprintf(os.Stderr, "registered %s (%s) — start with 'sindri agent start %s'\n", name, role, name)
					return nil
				}
				fmt.Fprintf(os.Stderr, "registered %s (%s) — starting it\n", name, role)
				// The launch streams (a first run builds the image, which is slow), and its own
				// output is the account of a failure. Registration already succeeded, so a
				// failed start is reported as exactly that — the agent exists and can be
				// started again, which a bare error would not convey.
				if err := b.Launch(name, false, false, os.Stderr); err != nil {
					return fmt.Errorf("%s was registered but did not start: %w\n"+
						"it exists as a stopped agent — retry with 'sindri agent start %s'", name, err, name)
				}
				return nil
			})
		},
	}
	c.Flags().StringVar(&role, "role", "worker", "agent role: worker|reviewer|planner|coauthor")
	c.Flags().StringVar(&memory, "memory", "", "RAM limit for this agent's container (e.g. 4g, 512m; unset = the runtime's default)")
	c.Flags().BoolVar(&noStart, "no-start", false, "register the identity only, without starting a container")
	return c
}

// agentMemoryCmd applies on next start: a running container's limit is fixed at creation.
func agentMemoryCmd() *cobra.Command {
	return &cobra.Command{
		Use: "memory <name> <size>", Short: "Set an agent's container RAM limit (e.g. 4g, 512m; 'default' to reset)", Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			size := args[1]
			if size == "default" {
				size = "" // reset to the hub default
			}
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				if err := b.SetMemory(a.Name, size); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "%s memory set to %s — restart it to apply ('sindri agent restart %s')\n",
					a.Name, args[1], a.Name)
				return nil
			})
		},
	}
}

func agentDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use: "delete <name>", Aliases: []string{"rm"}, Short: "Delete an agent (container, socket, worktree, identity), or remove an orphan", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			// An orphan is runtime with no roster entry, so there is no agent to look up and
			// withAgent would fail on it. Same command either way: the user sees one stray name
			// in `agent list` and should not have to know which kind of stray it is.
			removed, err := removeIfOrphan(args[0])
			if err != nil || removed {
				return err
			}
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				if err := b.DeleteAgent(a.Name); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "deleted %s\n", a.Name)
				return nil
			})
		},
	}
}

// removeIfOrphan removes name if the board lists it as an orphan, reporting whether it did. The
// board is what decides: an orphan is defined by having no roster entry, which only the hub knows.
func removeIfOrphan(name string) (bool, error) {
	done := false
	err := withBackend(func(b backend) error {
		st, err := b.State()
		if err != nil {
			return err
		}
		for _, o := range st.Orphans {
			if o != name {
				continue
			}
			if err := b.RemoveOrphan(name); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "removed orphan %s — it had no roster entry, so no agent was deleted\n", name)
			done = true
			return nil
		}
		return nil
	})
	return done, err
}

func agentPaneCmd() *cobra.Command {
	var lines int
	c := &cobra.Command{
		Use: "pane <name>", Short: "Print the agent's live tmux screen (capture-pane)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
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
	var shell, debug bool
	c := &cobra.Command{
		Use: "start <name>", Short: "Start the agent: spin a container that assumes its identity (runs Claude)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				// Launch already ends with "launched — coming up"; a "started" here would contradict it.
				return b.Launch(a.Name, shell, debug, os.Stderr)
			})
		},
	}
	c.Flags().BoolVar(&shell, "shell", false, "run a bare shell instead of Claude (debug/demo)")
	c.Flags().BoolVar(&debug, "debug", false, "stream the hub's liveness-probe detail while waiting for the agent to come up")
	return c
}

func agentStopCmd() *cobra.Command {
	return &cobra.Command{
		Use: "stop <name>", Short: "Tear down the agent's container (keeps its identity)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				if err := b.StopAgent(a.Name); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "stopped %s\n", a.Name)
				return nil
			})
		},
	}
}

// agentClearContextCmd sends /clear into a full agent's session — the confirmed remedy for
// retirement; the user typing this command IS the confirmation (the same convention `agent
// delete` uses for its own irreversible action, no extra prompt on top of it). The hub still
// refuses if the agent holds a task: clearing is only safe at a leaf boundary.
func agentClearContextCmd() *cobra.Command {
	return &cobra.Command{
		Use: "clear-context <name>", Short: "Send /clear into the agent's session and re-serve its directive (leaf boundary only)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				if err := b.ClearContext(a.Name); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "cleared %s's context — it will pick up its directive fresh\n", a.Name)
				return nil
			})
		},
	}
}

// agentRebaseCmd recovers a stale tree after the base moved outside a sindri merge;
// git aborts on conflict, so a failure changes nothing.
func agentRebaseCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rebase <name>", Short: "Rebase the agent's worktree onto the current reference branch", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				if err := b.RebaseAgent(a.Name); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "rebased %s onto the reference branch\n", a.Name)
				return nil
			})
		},
	}
}

// agentRebuildCmd re-pulls the base image and relaunches; the session resumes from the
// mounted home, so no conversation is lost.
func agentRebuildCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rebuild <name>", Short: "Rebuild the agent's image (re-pull the base) and relaunch it (session resumes)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				return b.RebuildImage(a.Name, os.Stderr) // streams build + restart progress
			})
		},
	}
}

// agentRestartCmd clears a wedged session; a down agent is just started, so it never errors.
func agentRestartCmd() *cobra.Command {
	var shell, debug bool
	c := &cobra.Command{
		Use: "restart <name>", Short: "Restart the agent's container (starts it if it wasn't running)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				if a.Status != "down" { // tear down the running container first
					if err := b.StopAgent(a.Name); err != nil {
						return err
					}
					fmt.Fprintf(os.Stderr, "stopped %s — relaunching…\n", a.Name)
				}
				// Launch streams progress and ends with "launched — coming up".
				return b.Launch(a.Name, shell, debug, os.Stderr)
			})
		},
	}
	c.Flags().BoolVar(&shell, "shell", false, "run a bare shell instead of Claude (debug/demo)")
	c.Flags().BoolVar(&debug, "debug", false, "stream the hub's liveness-probe detail while waiting for the agent to come up")
	return c
}

// agentDirCmd exists because a child can't cd the parent shell: `cd "$(sindri agent dir x)"`.
func agentDirCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dir <name>",
		Short: "Print an agent's workspace path — use: cd \"$(sindri agent dir <name>)\"",
		Long: "Print the absolute path to an agent's workspace. A command can't change " +
			"your shell's directory itself, so use it in a subshell:\n\n" +
			"  cd \"$(sindri agent dir <name>)\"\n\n" +
			"or add a shell function to your rc:\n\n" +
			"  scd() { cd \"$(sindri agent dir \"$1\")\"; }",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				st, err := b.State()
				if err != nil {
					return err
				}
				a := agentByName(st.Agents, args[0])
				if a == nil {
					return fmt.Errorf("no such agent %q", args[0])
				}
				root := projectRoot(st.Projects, a.Project)
				if root == "" || a.Workspace == "" {
					return fmt.Errorf("%s has no workspace yet (launch it / give it a task first)", a.Name)
				}
				fmt.Println(filepath.Join(root, a.Workspace))
				return nil
			})
		},
	}
}

func agentTellCmd() *cobra.Command {
	return &cobra.Command{
		Use: "tell <name> <message...>", Short: "Send a message into an agent's session ([user])", Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			msg := strings.Join(args[1:], " ")
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				if err := b.Tell(a.Name, msg, "user"); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "delivered to %s\n", a.Name)
				return nil
			})
		},
	}
}

// agentPlanCmd sends a phased brief — read, check for prior work, interview — not your raw text.
func agentPlanCmd() *cobra.Command {
	var taskID string
	c := &cobra.Command{
		Use: "plan <name> [what to plan...]", Short: "Assign a planner a plan to work out (reads, checks, then interviews you)",
		Long: "Assign a planner something to work out. It reads, checks for prior work, then interviews you.\n\n" +
			"--task hands it an existing task instead: the task's title and body are the brief, it\n" +
			"becomes the parent of every piece the planning produces, and it is held back from workers\n" +
			"until you rule on the result. Free text may accompany a task to add what the task omits.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			goal := strings.Join(args[1:], " ")
			if goal == "" && taskID == "" {
				return fmt.Errorf("say what to plan, or name a task with --task")
			}
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				if err := b.AssignPlan(a.Name, goal, taskID); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "assigned to %s — it will read, check for prior work, then interview you\n", a.Name)
				return nil
			})
		},
	}
	c.Flags().StringVar(&taskID, "task", "", "work up this existing task: it becomes the brief and the parent of what follows")
	return c
}

// agentTaskLabel is a task id with its title alongside it when known, else the bare id — so
// `agent info` says what an agent is working on, not just an id nobody recognizes. A lookup
// failure (another repo, task since scrapped) degrades to the bare id, not a command error.
func agentTaskLabel(b backend, id string) string {
	if id == "" {
		return dash(id)
	}
	if t, err := b.TaskInfo(id); err == nil && t.Title != "" {
		return id + "  " + t.Title
	}
	return id
}

func agentInfoCmd() *cobra.Command {
	var n int
	var debug bool
	c := &cobra.Command{
		Use: "info <name>", Short: "Show an agent's status (state, task, PR, clients, recent activity)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, found *api.AgentView) error {
				// The hub's own default, for the "(default)" note — read rather than assumed, since
				// it is the wired runtime's answer and differs between a container and a micro-VM.
				var dflt string
				if st, err := b.State(); err == nil {
					dflt = st.DefaultMemory
				}
				fmt.Printf("agent:     %s\nrole:      %s\nstatus:    %s\ntask:      %s\nfeature:   %s\npr:        %s\nworkspace: %s\nmemory:    %s\ncontext:   %s\n",
					found.Name, found.Role, found.Status, agentTaskLabel(b, found.Task),
					agentTaskLabel(b, found.Feature), dash(found.PR), dash(found.Workspace), memoryLabel(found.Memory, dflt),
					theme.ContextLine(found.ContextTokens))
				// engine + the exact runtime instance (id, image, cpus, memory limit, host pid)
				if inst, err := b.Instance(found.Name); err == nil && inst != "" {
					fmt.Printf("\n%s\n", inst)
				}
				if debug { // explain the status: what each liveness probe actually observes
					if d, err := b.Diagnose(found.Name); err == nil {
						fmt.Printf("\nliveness probe (why status is %q):\n%s", found.Status, d)
					}
				}
				if cs, err := b.Clients(found.Name); err == nil {
					fmt.Print(theme.FormatClients(cs))
				}
				evs, err := b.Log(found.Name)
				if err != nil {
					return err
				}
				// Status, not a log dump: the last n events, one capped line each (0 = all).
				total := len(evs)
				if n > 0 && total > n {
					evs = evs[total-n:]
				}
				fmt.Printf("\nrecent activity (%d of %d):\n", len(evs), total)
				for _, e := range evs {
					fmt.Printf("  %s  %-10s %s\n", eventTime(e.TS), e.Type, oneLine(e.Payload, 100))
				}
				return nil
			})
		},
	}
	c.Flags().IntVarP(&n, "num", "n", 8, "recent activity lines to show (0 = all)")
	c.Flags().BoolVar(&debug, "debug", false, "show what the hub's liveness probes observe (explains a puzzling status)")
	return c
}

// eventTime renders a UTC RFC3339 stamp as local HH:MM:SS, or raw if it won't parse.
func eventTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Local().Format("15:04:05")
}

// memoryLabel marks the fallback when unset, with the hub's own figure rather than a copy of it.
func memoryLabel(m, dflt string) string {
	if strings.TrimSpace(m) == "" {
		return theme.MemoryDefaultLabel(dflt)
	}
	return m
}

// shortAge renders an RFC3339 stamp's age as "3d"/"2h"/"5m"/"now"; "-" when unparseable.
func shortAge(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// oneLine caps a payload to its first line and max runes, keeping `info` one line per event.
func oneLine(s string, max int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimRight(s[:i], " ") + " …"
	}
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}
