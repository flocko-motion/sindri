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
	"path/filepath"
	"strings"
	"time"

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

// agentStatsCmd shows each running agent's VM memory usage against its limit — the
// view for tuning per-agent memory (how close each micro-VM is to its ceiling).
// Optional name arg narrows to one agent. Down agents are omitted (no VM to sample).
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
					var only []hub.AgentStatsView
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
					fmt.Printf("%-10.10s %-12s %s\n", v.Repo, v.Name, memLine(v.MemUsageBytes, v.MemLimitBytes))
				}
				return nil
			})
		},
	}
}

// memLine renders "544 MiB / 1024 MiB  53% [█████·····]" for a usage/limit pair.
func memLine(usage, limit int64) string {
	pct := 0.0
	if limit > 0 {
		pct = float64(usage) / float64(limit) * 100
	}
	return fmt.Sprintf("%9s / %-9s %3.0f%% %s", humanBytes(usage), humanBytes(limit), pct, memBar(pct))
}

// humanBytes formats a byte count in binary units (matches how memory limits are
// configured — 1024 MiB == the 1 GiB default).
func humanBytes(n int64) string {
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	f, units, i := float64(n), []string{"KiB", "MiB", "GiB", "TiB"}, -1
	for f >= u && i < len(units)-1 {
		f, i = f/u, i+1
	}
	return fmt.Sprintf("%.0f %s", f, units[i])
}

// memBar is a 10-cell usage meter; fuller = closer to the limit.
func memBar(pct float64) string {
	const w = 10
	fill := int(pct/100*w + 0.5)
	if fill > w {
		fill = w
	}
	if fill < 0 {
		fill = 0
	}
	return "[" + strings.Repeat("█", fill) + strings.Repeat("·", w-fill) + "]"
}

func agentNewCmd() *cobra.Command {
	var role, memory string
	c := &cobra.Command{
		Use: "new [name]", Short: "Register an agent identity (no container; name optional — auto dwarf name)", Args: cobra.MaximumNArgs(1),
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
				fmt.Fprintf(os.Stderr, "registered %s (%s) — start with 'sindri agent start %s'\n", name, role, name)
				return nil
			})
		},
	}
	c.Flags().StringVar(&role, "role", "worker", "agent role: worker|reviewer|planner|coauthor")
	c.Flags().StringVar(&memory, "memory", "", "RAM limit for this agent's container (e.g. 4g, 512m; default 2g)")
	return c
}

// agentMemoryCmd sets (or resets) an agent's RAM limit. Takes effect on the agent's
// next start/restart — a running container's limit is fixed when it's created.
func agentMemoryCmd() *cobra.Command {
	return &cobra.Command{
		Use: "memory <name> <size>", Short: "Set an agent's container RAM limit (e.g. 4g, 512m; 'default' to reset)", Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			size := args[1]
			if size == "default" {
				size = "" // reset to the hub default
			}
			return withAgent(args[0], func(b backend, a *hub.AgentView) error {
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
		Use: "delete <name>", Aliases: []string{"rm"}, Short: "Delete an agent (container, socket, worktree, identity)", Args: cobra.ExactArgs(1),
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
	var shell, debug bool
	c := &cobra.Command{
		Use: "start <name>", Short: "Start the agent: spin a container that assumes its identity (runs Claude)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *hub.AgentView) error {
				// Launch streams its build/start progress to stderr and ends with a
				// "launched — coming up" line; don't print a second, contradicting
				// "started" (the agent isn't live until the board says so).
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

// agentRebaseCmd rebases the agent's worktree onto the current base (reference)
// branch — for when the base moved outside a sindri merge and the agent is on a
// stale tree. git aborts on conflict, so a failure changes nothing and is reported.
func agentRebaseCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rebase <name>", Short: "Rebase the agent's worktree onto the current reference branch", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *hub.AgentView) error {
				if err := b.RebaseAgent(a.Name); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "rebased %s onto the reference branch\n", a.Name)
				return nil
			})
		},
	}
}

// agentRebuildCmd force-rebuilds the agent's container image (re-pulling the base,
// e.g. to pick up a newer Go) and relaunches the agent into it. The Claude session
// resumes from the mounted home, so the conversation isn't lost.
func agentRebuildCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rebuild <name>", Short: "Rebuild the agent's image (re-pull the base) and relaunch it (session resumes)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *hub.AgentView) error {
				return b.RebuildImage(a.Name, os.Stderr) // streams build + restart progress
			})
		},
	}
}

// agentRestartCmd stops the agent's pod and starts a fresh one — the way to pick
// up a rebuilt agent image or clear a wedged session. If the agent wasn't running,
// it's just a start (no error), so `restart` is always safe to reach for.
func agentRestartCmd() *cobra.Command {
	var shell, debug bool
	c := &cobra.Command{
		Use: "restart <name>", Short: "Restart the agent's container (starts it if it wasn't running)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *hub.AgentView) error {
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

// agentDirCmd prints an agent's workspace path. A child process can't change the
// parent shell's directory, so this is the composable primitive: `cd "$(sindri
// agent dir <name>)"`, or a shell function wrapping it. Read-only, so no cross-repo
// prompt — it just resolves the path.
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
	var n int
	var debug bool
	c := &cobra.Command{
		Use: "info <name>", Short: "Show an agent's status (state, task, PR, clients, recent activity)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, found *hub.AgentView) error {
				fmt.Printf("agent:     %s\nrole:      %s\nstatus:    %s\ntask:      %s\npr:        %s\nworkspace: %s\nmemory:    %s\n",
					found.Name, found.Role, found.Status, dash(found.Task), dash(found.PR), dash(found.Workspace), memoryLabel(found.Memory))
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
					fmt.Print(hub.FormatClients(cs))
				}
				evs, err := b.Log(found.Name)
				if err != nil {
					return err
				}
				// Status, not a log dump: show the last n events, each on one
				// timestamped, length-capped line. `-n 0` shows all.
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

// eventTime renders an activity timestamp (UTC RFC3339) as a local HH:MM:SS,
// falling back to the raw value if it doesn't parse.
func eventTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Local().Format("15:04:05")
}

// memoryLabel renders an agent's configured RAM limit, marking the hub fallback when
// none is set (the "2g" here mirrors the hub's defaultAgentMemory — display only).
func memoryLabel(m string) string {
	if strings.TrimSpace(m) == "" {
		return "2g (default)"
	}
	return m
}

// shortAge renders how long ago an RFC3339 timestamp was, compactly ("3d", "2h",
// "5m", "now"); "-" when empty or unparseable.
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

// oneLine collapses a possibly-multi-line payload to its first line, capped to max
// runes with an ellipsis — so `info` stays one line per event, not a log dump.
func oneLine(s string, max int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimRight(s[:i], " ") + " …"
	}
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}
