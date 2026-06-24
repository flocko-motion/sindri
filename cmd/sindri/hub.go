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
	"github.com/flo-at/sindri/internal/adapter/pod"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/spf13/cobra"
)

// backend is the full hub operation set; satisfied by both *hub.Hub (in-process,
// ephemeral) and *client.HTTP (a running hub over its socket).
type backend interface {
	NewAgent(name, role string) (string, error)
	DeleteAgent(name string) error
	StopAgent(name string) error
	AgentPane(name string, lines int) (string, error)
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

// open returns the running hub over its socket if one is up, else an ephemeral
// in-process hub.
func open(root string) (backend, error) {
	if hub.IsRunning(root) {
		return client.Dial(root), nil
	}
	return hub.New(root)
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

// --- agent ---

func newAgentCmd() *cobra.Command {
	c := &cobra.Command{Use: "agent", Short: "Manage agents (workers + reviewers)"}
	c.AddCommand(agentListCmd(), agentNewCmd(), agentDeleteCmd(), agentPaneCmd(), agentStartCmd(), agentStopCmd(), agentTellCmd(), agentAttachCmd(), agentInfoCmd())
	return c
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
					fmt.Printf("%-12s %-8s %-10s %-14s %s\n", a.Name, a.Role, a.Status, dash(a.Task), dash(a.PR))
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
	c.Flags().StringVar(&role, "role", "worker", "agent role: worker|reviewer|planner")
	return c
}

func agentDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use: "delete <name>", Aliases: []string{"rm"}, Short: "Delete an agent (pod, socket, worktree, identity)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.DeleteAgent(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "deleted %s\n", args[0])
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
			return withBackend(func(b backend) error {
				out, err := b.AgentPane(args[0], lines)
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
			root, err := repoRoot()
			if err != nil {
				return err
			}
			if !hub.IsRunning(root) {
				return fmt.Errorf("no hub running — start one first: 'sindri hub &' (agents need a persistent hub)")
			}
			cl := client.Dial(root)
			defer cl.Close()
			if err := cl.Launch(args[0], shell); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "started %s\n", args[0])
			return nil
		},
	}
	c.Flags().BoolVar(&shell, "shell", false, "run a bare shell instead of Claude (debug/demo)")
	return c
}

func agentStopCmd() *cobra.Command {
	return &cobra.Command{
		Use: "stop <name>", Short: "Tear down the agent's pod (keeps its identity)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.StopAgent(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "stopped %s\n", args[0])
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

func agentAttachCmd() *cobra.Command {
	var ro bool
	c := &cobra.Command{
		Use: "attach <name>", Short: "Attach to an agent's live tmux session (out-of-band)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			root, err := repoRoot()
			if err != nil {
				return err
			}
			c := hub.Container(root, name)
			if !pod.Running(c) {
				return fmt.Errorf("agent %q is not running", name)
			}
			return pod.ExecInteractive(c, append([]string{"tmux"}, tmux.Attach(name, ro)...)...)
		},
	}
	c.Flags().BoolVar(&ro, "read-only", false, "observe without typing")
	return c
}

func agentInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use: "info <name>", Short: "Show an agent's state and activity timeline", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				st, err := b.State()
				if err != nil {
					return err
				}
				var found *hub.AgentView
				for i := range st.Agents {
					if st.Agents[i].Name == args[0] {
						found = &st.Agents[i]
					}
				}
				if found == nil {
					return fmt.Errorf("no such agent %q", args[0])
				}
				fmt.Printf("agent:     %s\nrole:      %s\nstatus:    %s\ntask:      %s\npr:        %s\nworkspace: %s\n",
					found.Name, found.Role, found.Status, dash(found.Task), dash(found.PR), dash(found.Workspace))
				evs, err := b.Log(args[0])
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
	c.AddCommand(prListCmd(), prInfoCmd(), prReviewCmd(), prVerifyCmd(), prRejectCmd(), prLintCmd(), prMergeCmd(), prMilestoneCmd())
	return c
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
