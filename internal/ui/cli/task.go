// package: ui/cli / task
// type:    entrypoint (CLI command group)
// job:     the `sindri task …` verbs — list/info/new/edit/priority and the
// workflow actions (approve/reject/unassign/close). Each delegates to
// the hub via the shared backend (in-process or over the socket).
// limits:  no logic — argument plumbing only; the hub owns task semantics.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/theme"
	"github.com/spf13/cobra"
)

// --- task ---

// tasksJSON renders the task rows (their json tags) for machine consumers. It
// always yields a JSON array — never null — so the output parses even when there
// are no tasks.
func tasksJSON(tasks []api.Task) (string, error) {
	if tasks == nil {
		tasks = []api.Task{}
	}
	out, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// NewTaskCmd builds the `task` command tree (the backlog).
func NewTaskCmd() *cobra.Command {
	c := &cobra.Command{Use: "task", Short: "Inspect and create tasks"}
	c.AddCommand(taskListCmd(), taskInfoCmd(), taskNewCmd(), taskEditCmd(), taskPriorityCmd(), taskApproveCmd(), taskRejectCmd(), taskUnassignCmd(), taskCloseCmd(), taskDeleteCmd(), taskRefreshCmd(), taskCommentCmd())
	return c
}

// taskRefreshCmd re-syncs the task cache and notifies watchers. Reads sync on their own, so this is
// for forcing one without listing — e.g. pushing fresh state to a running TUI.
// taskCommentCmd comments on a task. A GitHub issue gets it upstream, so its own readers see it;
// every other kind keeps the thread in the hub.
func taskCommentCmd() *cobra.Command {
	return &cobra.Command{
		Use: "comment <id> <text...>", Short: "Comment on a task (a GitHub issue is commented upstream)",
		Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			body := strings.Join(args[1:], " ")
			return withBackend(func(b backend) error {
				if err := b.AddTaskComment(args[0], body); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "commented on %s\n", args[0])
				return nil
			})
		},
	}
}

func taskRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use: "refresh", Short: "Re-sync tasks from every source and notify watchers", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return withBackend(func(b backend) error {
				if err := b.Refresh(); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "synced tasks")
				return nil
			})
		},
	}
}

// taskCloseCmd marks a task done — dispatched by backend (td close / openspec archive
// / GitHub issue close).
func taskCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use: "close <id>", Short: "Close a task (done): td close · openspec archive · issue close", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.CloseTask(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "closed %s\n", args[0])
				return nil
			})
		},
	}
}

// taskDeleteCmd scraps a task, dispatched by backend. --subtasks widens it down the tree and --prs
// takes the open PRs, the same two shapes the TUI's scrap modal offers.
func taskDeleteCmd() *cobra.Command {
	var subtasks, prs bool
	c := &cobra.Command{
		Use: "delete <id>", Aliases: []string{"rm", "scrap"},
		Short: "Scrap a task (discard): td delete · openspec change removal · issue delete", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.ScrapTask(args[0], subtasks, prs); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "scrapped %s%s\n", args[0], scrapExtent(subtasks, prs))
				return nil
			})
		},
	}
	c.Flags().BoolVar(&subtasks, "subtasks", false, "scrap everything under the task too (children, theirs, …)")
	c.Flags().BoolVar(&prs, "prs", false, "scrap the open PR of every task scrapped")
	return c
}

// scrapExtent names how far a scrap reached, for the confirmation line.
func scrapExtent(subtasks, prs bool) string {
	switch {
	case subtasks && prs:
		return " with its subtasks and their PRs"
	case subtasks:
		return " with its subtasks"
	case prs:
		return " and its PR"
	}
	return ""
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
	var subtasks bool
	c := &cobra.Command{
		Use: "approve <id>", Short: "Approve a planner-proposed task (makes it claimable)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				id := args[0]
				// Counted before the write, so the number names what this call decided.
				pending := pendingBelow(b, id)
				if err := b.ApproveTask(id, subtasks); err != nil {
					return err
				}
				switch {
				case subtasks && pending > 0:
					fmt.Fprintf(os.Stderr, "approved %s and %s below it\n", id, plural(pending, "task", "tasks"))
				case pending > 0:
					// The TUI offers the wider approve in a modal; here the flag is the way to it.
					fmt.Fprintf(os.Stderr, "approved %s — %s below it still await approval (--subtasks takes them too)\n",
						id, plural(pending, "task", "tasks"))
				default:
					fmt.Fprintf(os.Stderr, "approved %s\n", id)
				}
				return nil
			})
		},
	}
	c.Flags().BoolVar(&subtasks, "subtasks", false, "approve every task below it that still awaits a verdict")
	return c
}

// pendingBelow counts the tasks under id still awaiting a verdict; 0 if the backlog can't be read,
// since a missing count must not turn an approve into a failure.
func pendingBelow(b backend, id string) int {
	all, err := b.Tasks()
	if err != nil {
		return 0
	}
	return len(api.PendingApproval(all, id))
}

// plural renders a counted noun for the confirmation lines.
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
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
		Use:   "priority <id> <critical|high|mid|low|none>",
		Short: "Set a task's priority (a P-code; openspec and GitHub items keep theirs in our db)",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.SetPriority(args[0], theme.PriorityCode(args[1])); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "set %s priority %s\n", args[0], args[1])
				return nil
			})
		},
	}
}

// taskState is the word a listing shows for a task: the approval gate where one is set, since that
// is what decides whether the task can be worked, and the status otherwise. A pending task printed
// as plain "open" claimed to be available when no worker could see it.
func taskState(t api.Task) string {
	if t.Approval == "pending" || t.Approval == "rejected" {
		return t.Approval
	}
	return t.Status
}

func taskListCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use: "list", Short: "List tasks", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return withBackend(func(b backend) error {
				tasks, err := b.Tasks()
				if err != nil {
					return err
				}
				if asJSON {
					out, err := tasksJSON(tasks)
					if err != nil {
						return err
					}
					fmt.Println(out)
					return nil
				}
				for _, t := range tasks {
					fmt.Printf("%-12s %-8s %-12s %s\n", t.ID, theme.PriorityLabel(t.Priority), taskState(t), t.Title)
				}
				if len(tasks) == 0 {
					fmt.Fprintln(os.Stderr, "no tasks")
				}
				// The gate hides these from every worker, so a list that ended here read as a full
				// backlog while nothing in it could be claimed.
				if n := api.CountAwaitingVerdict(tasks); n > 0 {
					fmt.Fprintf(os.Stderr, "\n%d task(s) await your verdict and no worker can claim them: "+
						"`sindri task approve <id>` (--subtasks clears the tree below it), or `sindri task reject <id> <why>`.\n", n)
				}
				return nil
			})
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "output tasks as JSON (machine-readable) instead of the table")
	return c
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
				// The same fields the TUI pane and the agent's `task <id>` show: a front-end
				// chooses layout, not which facts exist, or it answers a different question.
				fmt.Printf("id:       %s\ntitle:    %s\nstatus:   %s\ntype:     %s\npriority: %s\nparent:   %s\napproval: %s\nlabels:   %s\nurl:      %s\n",
					t.ID, t.Title, t.Status, dash(t.Type), theme.PriorityLabel(t.Priority),
					dash(t.ParentID), dash(t.Approval), dash(t.Labels), dash(t.URL))
				if body := strings.TrimRight(t.Description, "\n"); body != "" {
					fmt.Printf("\n%s\n", body)
				}
				// The thread too, for the same reason the fields above are all here: the TUI's pane
				// shows it, so a CLI that omitted it answered a different question.
				for _, c := range t.Comments {
					fmt.Printf("\n— %s (%s, %s)\n%s\n", dash(c.Author), c.Source, c.CreatedAt,
						strings.TrimRight(c.Body, "\n"))
				}
				return nil
			})
		},
	}
}

func taskNewCmd() *cobra.Command {
	var typ, priority, parent, labels, desc string
	c := &cobra.Command{
		Use: "new <title...>", Short: "Create a task", Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				id, err := b.CreateTask(api.TaskSpec{
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
				if err := b.EditTask(args[0], api.TaskSpec{
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
