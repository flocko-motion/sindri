// package: ui/cli / commands
// type:    command (host CLI)
// job:     the host command tree — hierarchical <category> <action>: agent
//          {list,new,launch,tell,attach,info}, task {list,new,info}, pr
//          {list,info,merge}; plus first-order hub. Every hub capability has a
//          CLI verb so functionality is verifiable from the shell, not only the
//          TUI.
// limits:  no logic — each verb is a thin call into a backend (in-process hub
//          when none is running, the socket client otherwise).
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// backend is the full hub operation set the host CLI uses; satisfied by
// *client.HTTP (the single global hub over its socket — repo context rides the
// client's X-Sindri-Project header, so these signatures don't carry a project).
type backend interface {
	NewAgent(name, role, memory string) (string, error)
	SetMemory(name, memory string) error
	DeleteAgent(name string) error
	StopAgent(name string) error
	RebaseAgent(name string) error
	RebuildImage(name string, out io.Writer) error
	AgentPane(name string, lines int) (string, error)
	Diagnose(name string) (string, error)
	Stats() (hub.StatsReport, error)
	Instance(name string) (string, error)
	Clients(name string) ([]hub.ClientView, error)
	Launch(name string, shell, debug bool, out io.Writer) error
	Tell(name, msg, source string) error
	ChatAdd(name string) error
	ChatRemove(name string) error
	ChatSay(msg string) error
	ChatHeartbeat() error
	Chat() (hub.ChatView, error)
	ChatWatch(ctx context.Context) (<-chan hub.ChatView, error)
	State() (hub.BoardState, error)
	Log(name string) ([]store.Event, error)
	Tasks() ([]store.Task, error)
	TaskInfo(id string) (store.Task, error)
	CreateTask(s hub.TaskSpec) (string, error)
	EditTask(id string, s hub.TaskSpec) error
	SetPriority(id, priority string) error
	ApproveTask(id string) error
	RejectTask(id, comment string) error
	UnassignTask(id string) error
	CloseTask(id string) error
	DeleteTask(id string) error
	Refresh() error
	PRs() ([]store.PR, error)
	PRInfo(id string) (hub.PRDetail, error)
	RejectPR(id, feedback string) error
	ApprovePR(id string) error
	LintPR(id string) (string, error)
	RequestReview(id, requirement string) error
	MaterializeReview(id string) (string, error)
	Merge(id string) (store.PR, error)
	MilestonePR(agent string) (store.PR, error)
	Repos() ([]hub.RepoSummary, error)
	RepoInfo(tag string) (hub.RepoDetail, error)
	RepoInit() (hub.RepoSummary, error)
	RepoForget(tag string) error
	SetRepoColor(tag string, color int) error
	RemoveOrphan(name string) error
	WriteRepoConfig(cfg config.Config) error
	Close() error
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return git.Root(wd)
}

// open connects to the single global hub for a host command, auto-starting it if
// needed. There's no ephemeral in-process backend anymore — one hub serves every
// repo, and it's cheap to keep running.
func open(root string) (backend, error) {
	if err := ensureHubRunning(); err != nil {
		return nil, err
	}
	return dialHub(root)
}

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

// --- first-order: hub ---

// NewHubCmd builds the `hub` command tree (start/stop/status the hub).
func NewHubCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "hub",
		Short: "Manage the global hub service (start, restart, status, stop)",
		Long: "The hub is the single global coordinator that drives the agents across\n" +
			"every repo.\n\n" +
			"  sindri hub start        run the hub in the foreground\n" +
			"  sindri hub start --bg   run it in the background (same as `sindri hub start &`)\n" +
			"  sindri hub restart      stop it and start a fresh detached one (pick up a rebuild)\n" +
			"  sindri hub status       show the running hub (pid, version, uptime)\n" +
			"  sindri hub stop         stop the running hub",
	}
	c.AddCommand(newHubStartCmd(), newHubRestartCmd(), newHubStatusCmd(), newHubStopCmd())
	return c
}

func newHubStartCmd() *cobra.Command {
	var bg bool
	c := &cobra.Command{
		Use:   "start",
		Short: "Run this repo's hub in the foreground (--bg to run it in the background)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if hub.IsRunning() {
				// A hub is already up. Same build → nothing to do; a different (or
				// unknown, pre-stamp) build → offer to take over.
				_, ver, ok := hub.ReadPID()
				if ok && ver == version {
					return fmt.Errorf("a hub is already running (%s)", hub.SocketPath())
				}
				desc := "an older build (predates version stamping)"
				if ok {
					desc = "sindri " + ver
				}
				fmt.Fprintf(os.Stderr, "a hub (%s) is already running; this CLI is %s.\n", desc, version)
				if !term.IsTerminal(int(os.Stdin.Fd())) || !promptYesNo("stop it and start this one?") {
					return fmt.Errorf("a hub is already running (%s)", hub.SocketPath())
				}
				pid, havePID := hub.HubPID()
				if !havePID {
					return fmt.Errorf("couldn't find the running hub's pid to stop it — stop it manually, then re-run")
				}
				if err := stopHub(pid); err != nil {
					return err
				}
			}
			if bg {
				return startHub() // detached; returns once the socket answers
			}
			h, err := hub.New()
			if err != nil {
				return err
			}
			defer h.Close()
			// Stamp this process (pid + build version) as the hub, so a second hub
			// can't start and clients can detect a stale-version hub.
			if err := hub.WritePID(version); err != nil {
				return err
			}
			defer hub.RemovePID()
			fmt.Fprintf(os.Stderr, "sindri hub listening at %s\n", h.SocketPath())
			return h.Serve()
		},
	}
	c.Flags().BoolVar(&bg, "bg", false, "run the hub detached in the background instead of the foreground")
	return c
}

// newHubRestartCmd stops the running hub and starts a fresh detached one — the way
// to pick up a rebuilt binary without a manual stop-then-start. If no hub is
// running it's just a start, so `restart` is always safe to reach for (mirrors
// `sindri agent restart`). Agents keep running across the restart; only the
// coordinator process is replaced.
func newHubRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the hub in the background (starts one if none is running)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !hub.IsRunning() {
				fmt.Fprintln(os.Stderr, "no hub running — starting a fresh one.")
				return startHub()
			}
			pid, ok := hub.HubPID()
			if !ok {
				return fmt.Errorf("couldn't find the running hub's pid to restart it — stop it manually, then `sindri hub start --bg`")
			}
			return restartHub(pid)
		},
	}
}

// --- agent ---

// NewAgentCmd builds the `agent` command tree (manage agents).
func NewAgentCmd() *cobra.Command {
	c := &cobra.Command{Use: "agent", Short: "Manage agents (workers, reviewers, planners, coauthors)",
		PersistentPreRun: agentPreflight} // warn up front if podman is down — nothing works without it
	c.AddCommand(agentListCmd(), agentStatsCmd(), agentNewCmd(), agentDeleteCmd(), agentPaneCmd(), agentStartCmd(), agentStopCmd(), agentRestartCmd(), agentRebaseCmd(), agentRebuildCmd(), agentMemoryCmd(), agentTellCmd(), agentDirCmd(), agentAttachCmd(), agentInfoCmd())
	return c
}

// --- pr ---

// NewPrCmd builds the `pr` command tree (review/merge pull requests).
func NewPrCmd() *cobra.Command {
	c := &cobra.Command{Use: "pr", Short: "Inspect and merge pull requests (merge-intents)"}
	c.AddCommand(prListCmd(), prInfoCmd(), prReviewCmd(), prVerifyCmd(), prApproveCmd(), prRejectCmd(), prLintCmd(), prMergeCmd(), prMilestoneCmd())
	return c
}

// prApproveCmd is the human approve: mark an open PR approved so it can be merged
// without a reviewer agent (the positive counterpart of pr reject).
func prApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use: "approve <pr-id>", Short: "Approve an open PR yourself (no reviewer agent needed), so it can be merged", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.ApprovePR(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "%s approved — merge it with 'sindri pr merge %s'\n", args[0], args[0])
				return nil
			})
		},
	}
}

// prMilestoneCmd opens a milestone PR for the container an agent is collaborating
// on: it captures the feature branch's current state and blocks the agent until
// you review and merge it; the agent then resumes the same container.
func prMilestoneCmd() *cobra.Command {
	return &cobra.Command{
		Use: "milestone <agent>", Short: "Open a milestone PR for the feature an agent is working (blocks it until merged)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				pr, err := b.MilestonePR(args[0])
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "opened milestone %s on %s\n", pr.ID, pr.Branch)
				return nil
			})
		},
	}
}

func prVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use: "verify <pr-id>", Short: "Check the PR out into the review workspace for hands-on inspection", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				path, err := b.MaterializeReview(args[0])
				if err != nil {
					return err
				}
				fmt.Println(path) // the worktree path — cd there to inspect
				return nil
			})
		},
	}
}

func prReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use: "review <pr-id> [requirement...]", Short: "Request an agentic review of a PR (assigns a reviewer agent)", Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.RequestReview(args[0], strings.Join(args[1:], " ")); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "review requested for %s\n", args[0])
				return nil
			})
		},
	}
}

func prRejectCmd() *cobra.Command {
	return &cobra.Command{
		Use: "reject <pr-id> <feedback...>", Short: "Reject a PR with feedback (routed to the worker)", Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.RejectPR(args[0], strings.Join(args[1:], " ")); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "rejected %s\n", args[0])
				return nil
			})
		},
	}
}

func prLintCmd() *cobra.Command {
	return &cobra.Command{
		Use: "lint <pr-id>", Short: "Run the quality gate against a PR's worktree", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				out, err := b.LintPR(args[0])
				if err != nil {
					return err
				}
				fmt.Print(out)
				return nil
			})
		},
	}
}

func prListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List PRs", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return withBackend(func(b backend) error {
				prs, err := b.PRs()
				if err != nil {
					return err
				}
				for _, p := range prs {
					fmt.Printf("%-14s %-12s %4s  %-10s %s\n", p.ID, p.Status, shortAge(p.CreatedAt), p.Agent, p.Branch)
				}
				if len(prs) == 0 {
					fmt.Fprintln(os.Stderr, "no PRs")
				}
				return nil
			})
		},
	}
}

func prInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use: "info <pr-id>", Short: "Show a PR and its diff", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				d, err := b.PRInfo(args[0])
				if err != nil {
					return err
				}
				p := d.PR
				fmt.Printf("%s  [%s]  by %s\nbranch %s → %s\n", p.ID, p.Status, p.Agent, p.Branch, p.Base)
				if p.Feedback != "" {
					fmt.Printf("feedback: %s\n", p.Feedback)
				}
				fmt.Printf("\n%s\n", strings.TrimSpace(d.Diff))
				return nil
			})
		},
	}
}

func prMergeCmd() *cobra.Command {
	return &cobra.Command{
		Use: "merge <pr-id>", Short: "Merge an approved PR (human-only — the hard gate)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				pr, err := b.Merge(args[0])
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "merged %s (task %s) → %s\n", pr.ID, pr.Task, pr.Base)
				return nil
			})
		},
	}
}
