// package: ui/cli / task
// type:    entrypoint (CLI command group)
// job:     the `sindri task …` verbs — list/info/new/edit/priority and the
//          workflow actions (approve/reject/unassign/close). Each delegates to
//          the hub via the shared backend (in-process or over the socket).
// limits:  no logic — argument plumbing only; the hub owns task semantics.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/spf13/cobra"
)

// --- task ---

// tasksJSON renders the task rows (their json tags) for machine consumers. It
// always yields a JSON array — never null — so the output parses even when there
// are no tasks.
func tasksJSON(tasks []store.Task) (string, error) {
	if tasks == nil {
		tasks = []store.Task{}
	}
	out, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// NewTaskCmd builds the `task` command tree (the backlog).
func NewTaskCmd() *cobra.Command {
	c := &cobra.Command{Use: "task", Short: "Inspect and create tasks (td issues)"}
	c.AddCommand(taskListCmd(), taskInfoCmd(), taskNewCmd(), taskEditCmd(), taskPriorityCmd(), taskApproveCmd(), taskRejectCmd(), taskUnassignCmd(), taskCloseCmd(), taskDeleteCmd(), taskRefreshCmd())
	return c
}

// taskRefreshCmd re-syncs the hub's task cache from td (the source of truth) and
// notifies watchers — the shell counterpart of the TUI's refresh, so the sync is
// reachable from the CLI too, not only the dashboard. Reads (list/info) already
// sync on their own; this is for forcing a refresh (e.g. to push fresh state to a
// running TUI) without listing.
func taskRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use: "refresh", Short: "Re-sync tasks from td (the source of truth) and notify watchers", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return withBackend(func(b backend) error {
				if err := b.Refresh(); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "synced tasks from td")
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

// taskDeleteCmd scraps a task — dispatched by backend (td delete / openspec change-dir
// removal / GitHub issue delete).
func taskDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use: "delete <id>", Aliases: []string{"rm", "scrap"},
		Short: "Scrap a task (discard): td delete · openspec change removal · issue delete", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.DeleteTask(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "scrapped %s\n", args[0])
				return nil
			})
		},
	}
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
		Use:   "priority <id> <critical|high|mid|low|none>",
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
					fmt.Printf("%-12s %-8s %-12s %s\n", t.ID, hub.PriorityLabel(t.Priority), t.Status, t.Title)
				}
				if len(tasks) == 0 {
					fmt.Fprintln(os.Stderr, "no tasks")
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
