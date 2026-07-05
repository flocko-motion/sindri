// package: main (sindri) / commands
// type:    command (host CLI)
// job:     the host command tree — hierarchical <category> <action>: agent
//
//	{list,new,launch,tell,attach,info}, task {list,new,info}, pr
//	{list,info,merge}; plus first-order hub. Every hub capability has a
//	CLI verb so functionality is verifiable from the shell, not only the
//	TUI.
//
// limits:  no logic — each verb is a thin call into a backend (in-process hub
//
//	when none is running, the socket client otherwise).
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// backend is the full hub operation set the host CLI uses; satisfied by
// *client.HTTP (the single global hub over its socket — repo context rides the
// client's X-Sindri-Project header, so these signatures don't carry a project).
type backend interface {
	NewAgent(name, role string) (string, error)
	DeleteAgent(name string) error
	StopAgent(name string) error
	AgentPane(name string, lines int) (string, error)
	Clients(name string) ([]hub.ClientView, error)
	Launch(name string, shell bool) error
	Tell(name, msg, source string) error
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
	PRs() ([]store.PR, error)
	PRInfo(id string) (hub.PRDetail, error)
	RejectPR(id, feedback string) error
	ApprovePR(id string) error
	LintPR(id string) (string, error)
	RequestReview(id, requirement string) error
	MaterializeReview(id string) (string, error)
	Merge(id string) (store.PR, error)
	MilestonePR(agent string) (store.PR, error)
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

func newHubCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "hub",
		Short: "Manage the per-repo hub service (start, list, stop)",
		Long: "The hub is the single global coordinator that drives the agents across\n" +
			"every repo.\n\n" +
			"  sindri hub start        run the hub in the foreground\n" +
			"  sindri hub start --bg   run it in the background (same as `sindri hub start &`)\n" +
			"  sindri hub status       show the running hub (pid, version, uptime)\n" +
			"  sindri hub stop         stop the running hub",
	}
	c.AddCommand(newHubStartCmd(), newHubStatusCmd(), newHubStopCmd())
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

// --- agent ---

func newAgentCmd() *cobra.Command {
	c := &cobra.Command{Use: "agent", Short: "Manage agents (workers, reviewers, planners, coauthors)",
		PersistentPreRun: agentPreflight} // warn up front if podman is down — nothing works without it
	c.AddCommand(agentListCmd(), agentNewCmd(), agentDeleteCmd(), agentPaneCmd(), agentStartCmd(), agentStopCmd(), agentRestartCmd(), agentTellCmd(), agentAttachCmd(), agentInfoCmd())
	return c
}

// --- task ---

func newTaskCmd() *cobra.Command {
	c := &cobra.Command{Use: "task", Short: "Inspect and create tasks (td issues)"}
	c.AddCommand(taskListCmd(), taskInfoCmd(), taskNewCmd(), taskEditCmd(), taskPriorityCmd(), taskApproveCmd(), taskRejectCmd(), taskUnassignCmd())
	return c
}

func taskUnassignCmd() *cobra.Command {
	return &cobra.Command{
		Use: "unassign <id>", Short: "Release a task back to the backlog (refused if a live agent holds it)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.UnassignTask(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "unassigned %s\n", args[0])
				return nil
			})
		},
	}
}

func taskApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use: "approve <id>", Short: "Approve a planner-proposed task (makes it claimable)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.ApproveTask(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "approved %s\n", args[0])
				return nil
			})
		},
	}
}

func taskRejectCmd() *cobra.Command {
	return &cobra.Command{
		Use: "reject <id> <comment...>", Short: "Reject a planner-proposed task with a comment", Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.RejectTask(args[0], strings.Join(args[1:], " ")); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "rejected %s\n", args[0])
				return nil
			})
		},
	}
}

func taskPriorityCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "priority <id> <critical|high|mid|low|minor>",
		Short: "Set a task's priority (td tasks → td; openspec items → our db)",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.SetPriority(args[0], hub.PriorityCode(args[1])); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "set %s priority %s\n", args[0], args[1])
				return nil
			})
		},
	}
}

func taskListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List tasks", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return withBackend(func(b backend) error {
				tasks, err := b.Tasks()
				if err != nil {
					return err
				}
				for _, t := range tasks {
					fmt.Printf("%-12s %-8s %-12s %s\n", t.ID, hub.PriorityLabel(t.Priority), t.Status, t.Title)
				}
				if len(tasks) == 0 {
					fmt.Fprintln(os.Stderr, "no tasks")
				}
				return nil
			})
		},
	}
}

func taskInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use: "info <id>", Short: "Show a task", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				t, err := b.TaskInfo(args[0])
				if err != nil {
					return err
				}
				fmt.Printf("id:       %s\ntitle:    %s\nstatus:   %s\ntype:     %s\npriority: %s\nlabels:   %s\n",
					t.ID, t.Title, t.Status, dash(t.Type), hub.PriorityLabel(t.Priority), dash(t.Labels))
				return nil
			})
		},
	}
}

func taskNewCmd() *cobra.Command {
	var typ, priority, parent, labels, desc string
	c := &cobra.Command{
		Use: "new <title...>", Short: "Create a task (a td issue)", Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				id, err := b.CreateTask(hub.TaskSpec{
					Title: strings.Join(args, " "), Type: typ, Priority: priority,
					Parent: parent, Description: desc, Labels: splitCSV(labels),
				})
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "created %s\n", id)
				return nil
			})
		},
	}
	taskSpecFlags(c, &typ, &priority, &parent, &labels, &desc)
	return c
}

func taskEditCmd() *cobra.Command {
	var typ, priority, parent, labels, desc, title string
	c := &cobra.Command{
		Use: "edit <id>", Short: "Edit a task (only the flags you pass are changed)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.EditTask(args[0], hub.TaskSpec{
					Title: title, Type: typ, Priority: priority,
					Parent: parent, Description: desc, Labels: splitCSV(labels),
				}); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "edited %s\n", args[0])
				return nil
			})
		},
	}
	c.Flags().StringVar(&title, "title", "", "new title")
	taskSpecFlags(c, &typ, &priority, &parent, &labels, &desc)
	return c
}

func taskSpecFlags(c *cobra.Command, typ, priority, parent, labels, desc *string) {
	c.Flags().StringVarP(typ, "type", "t", "", "issue type: bug, feature, task, epic, chore (default: task)")
	c.Flags().StringVarP(priority, "priority", "p", "", "priority: P0, P1, P2, P3, P4 (P0 highest; high/medium/low also accepted)")
	c.Flags().StringVar(parent, "parent", "", "parent task id (creates a child)")
	c.Flags().StringVarP(desc, "desc", "d", "", "description body")
	c.Flags().StringVar(labels, "labels", "", "comma-separated labels")
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// --- pr ---

func newPrCmd() *cobra.Command {
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
					fmt.Printf("%-14s %-9s %-10s %s\n", p.ID, p.Status, p.Agent, p.Branch)
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
