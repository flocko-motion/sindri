// package: ui/cli / run
// type:    entrypoint (CLI command group)
// job:     the `sindri run …` verbs — list/info/output/cancel/priority. Each
// delegates to the hub via the shared backend (in-process or over the socket).
// limits:  no logic — argument plumbing only; the hub owns the run queue.
package cli

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/api"
	"github.com/spf13/cobra"
)

// NewRunCmd builds the `run` command tree (the scheduled-run queue).
func NewRunCmd() *cobra.Command {
	c := &cobra.Command{Use: "run", Short: "Inspect and manage scheduled runs (the run queue)"}
	c.AddCommand(runListCmd(), runInfoCmd(), runOutputCmd(), runCancelCmd(), runPriorityCmd())
	return c
}

// runStatusLabel adds the queue position to a queued run's status, the same way api.StatusLabel
// decorates an approved PR's — the position is what "queued" alone never says.
func runStatusLabel(r api.Run) string {
	if r.Status == "queued" && r.Position > 0 {
		return fmt.Sprintf("queued(#%d)", r.Position)
	}
	return r.Status
}

func runListCmd() *cobra.Command {
	var filter string
	c := &cobra.Command{
		Use: "list", Short: "List runs", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			f, err := api.ParseRunFilter(filter)
			if err != nil {
				return err
			}
			return withBackend(func(b backend) error {
				all, err := b.Runs()
				if err != nil {
					return err
				}
				runs := api.FilterRuns(f, all)
				for _, r := range runs {
					fmt.Printf("%-14s %-12s %4s  %-10s %s\n",
						r.ID, runStatusLabel(r), shortAge(r.CreatedAt), r.Agent, r.Command)
				}
				if n := len(all) - len(runs); n > 0 {
					fmt.Fprintf(os.Stderr, "(filter %s — %d of %d run(s) shown)\n", f, len(runs), len(all))
				} else if len(runs) == 0 {
					fmt.Fprintln(os.Stderr, "no runs")
				}
				return nil
			})
		},
	}
	// Defaults to "all", the same reasoning prListCmd/taskListCmd give: a listing is a record,
	// not the TUI's redrawn view, which opens on "active" instead.
	c.Flags().StringVar(&filter, "filter", string(api.RunFilterAll),
		"which runs to list: "+api.RunFilterNames()+" (active = open, plus anything closed within "+
			api.ActiveWindow.String()+")")
	return c
}

func runInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use: "info <run-id>", Short: "Show a run's status, agent, command and timing", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				d, err := b.RunInfo(args[0])
				if err != nil {
					return err
				}
				r := d.Run
				fmt.Printf("%s  [%s]  by %s\ncommand: %s\n", r.ID, runStatusLabel(r), r.Agent, r.Command)
				if r.Priority != "" {
					fmt.Printf("priority: %s\n", r.Priority)
				}
				fmt.Printf("created: %s\n", r.CreatedAt)
				if r.StartedAt != "" {
					fmt.Printf("started: %s\n", r.StartedAt)
				}
				if r.FinishedAt != "" {
					fmt.Printf("finished: %s\n", r.FinishedAt)
				}
				return nil
			})
		},
	}
}

func runOutputCmd() *cobra.Command {
	return &cobra.Command{
		Use: "output <run-id>", Short: "Print a run's stored console output (capped)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				d, err := b.RunInfo(args[0])
				if err != nil {
					return err
				}
				if d.Output == "" {
					fmt.Fprintln(os.Stderr, "no output yet")
					return nil
				}
				fmt.Print(d.Output)
				return nil
			})
		},
	}
}

func runCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use: "cancel <run-id>", Short: "Cancel a queued or running run", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.CancelRun(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "cancelled %s\n", args[0])
				return nil
			})
		},
	}
}

func runPriorityCmd() *cobra.Command {
	return &cobra.Command{
		Use: "priority <run-id> <P0..P4>", Short: "Reprioritise a queued run", Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.ReprioritiseRun(args[0], args[1]); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "%s reprioritised to %s\n", args[0], args[1])
				return nil
			})
		},
	}
}
