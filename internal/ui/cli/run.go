// package: ui/cli / run
// type:    entrypoint (CLI command group)
// job:     the `sindri run …` verbs — list/info/output/cancel/priority. Each
// delegates to the hub via the shared backend (in-process or over the socket).
// limits:  no logic — argument plumbing only; the hub owns the run queue.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/table"
	"github.com/spf13/cobra"
)

// NewRunCmd builds the `run` command tree (the scheduled-run queue).
func NewRunCmd() *cobra.Command {
	c := &cobra.Command{Use: "run", Short: "Queue, inspect and manage scheduled runs (the run queue)"}
	c.AddCommand(runNewCmd(), runListCmd(), runInfoCmd(), runOutputCmd(), runCancelCmd(), runPriorityCmd())
	return c
}

// runNewCmd queues a run as the user, into the same single slot agents share. The target is named,
// never taken from the working directory: the same command means different things in different
// trees, and a wrong guess spends the fleet's only slot on it.
func runNewCmd() *cobra.Command {
	var agent, priority, timeout string
	c := &cobra.Command{
		Use:   "new <command...>",
		Short: "Queue a command to run against this repo's checkout, or an agent's workspace",
		Long: "Queues a shell command into the fleet's single run slot — the same queue agents use, so\n" +
			"a suite you want run does not race whatever an agent is already running.\n\n" +
			"It runs against a COPY of the target, made when the run reaches the front, so nothing the\n" +
			"command writes reaches the tree you are working in. Without --agent the target is this\n" +
			"repo's own checkout, uncommitted work included — which is the point: it tests what you\n" +
			"have, not what you have committed.\n\n" +
			"A run you queue goes ahead of every agent's, including their submit gates: you are sitting\n" +
			"there waiting on it and they are not. `sindri run priority` re-orders it afterwards.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if priority != "" {
				code, ok := api.ParsePriority(priority)
				if !ok {
					return fmt.Errorf("unknown priority %q — one of: %s", priority, strings.Join(api.PriorityWords, ", "))
				}
				priority = code
			}
			return withBackend(func(b backend) error {
				r, err := b.ScheduleRun(strings.Join(args, " "), agent, priority, timeout)
				if err != nil {
					return err
				}
				target := "this repo's checkout"
				if agent != "" {
					target = agent + "'s workspace"
				}
				fmt.Println(r.ID)
				fmt.Fprintf(os.Stderr, "queued against %s — `sindri run list` for its place in line, "+
					"`sindri run output %s` for the result.\n", target, r.ID)
				return nil
			})
		},
	}
	c.Flags().StringVar(&agent, "agent", "", "run against this agent's workspace instead of the repo's checkout")
	c.Flags().StringVar(&priority, "priority", "", "order among queued runs: "+strings.Join(api.PriorityWords, ", "))
	c.Flags().StringVar(&timeout, "timeout", "", "narrow the run's budget (a duration, e.g. 5m); never widens the hub's cap")
	return c
}

// runRequester names who asked for a run. A user's reads "you" rather than the bare sentinel: the
// column is otherwise a list of agent names with one word in it that looks like another agent.
func runRequester(r api.Run) string {
	if api.RunFromUser(r) {
		return "you"
	}
	return r.Agent
}

// runStatusLabel adds the queue position to a queued run's status, the same way api.StatusLabel
// decorates an approved PR's — the position is what "queued" alone never says.
func runStatusLabel(r api.Run) string {
	if r.Status == "queued" && r.Position > 0 {
		return fmt.Sprintf("queued(#%d)", r.Position)
	}
	return r.Status
}

// runListTable is the columns `sindri run list` prints, its header and its rows alike.
var runListTable = table.Table{
	{Label: "run", Width: 14},
	{Label: "status", Width: 12},
	{Label: "age", Width: 4, Right: true},
	{Label: "queued by", Width: 10},
	{Label: "command"},
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
				lines := make([]string, 0, len(runs))
				for _, r := range runs {
					lines = append(lines, runListTable.Line(
						table.Cell{Text: r.ID},
						table.Cell{Text: runStatusLabel(r)},
						table.Cell{Text: shortAge(r.CreatedAt)},
						table.Cell{Text: runRequester(r)},
						table.Cell{Text: r.Command},
					))
				}
				printRows(runListTable, lines)
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
