package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/flo-at/sindri/internal/worker"
	"github.com/spf13/cobra"
)


func main() {
	var projectDir string
	rootCmd := &cobra.Command{
		Use:   "sindri",
		Short: "Sindri — AI agent orchestrator",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if projectDir != "" {
				return os.Chdir(projectDir)
			}
			return nil
		},
	}
	rootCmd.PersistentFlags().StringVar(&projectDir, "project", "", "Project directory (default: git root from cwd)")

	workerCmd := &cobra.Command{
		Use:   "worker",
		Short: "Manage Sindri workers",
	}

	workerListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all workers and their status",
		RunE:  runWorkerList,
	}

	var skillName string
	var shellMode bool
	workerStartCmd := &cobra.Command{
		Use:   "start [name]",
		Short: "Start a worker interactively (creates worktree if needed)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkerStart(args, skillName, shellMode)
		},
	}
	workerStartCmd.Flags().StringVar(&skillName, "skill", "", "Skill to run (e.g. td-next, td-review)")
	workerStartCmd.Flags().BoolVar(&shellMode, "shell", false, "Open a shell instead of launching claude (for debugging)")

	workerStopCmd := &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a running worker container",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkerStop,
	}

	workerResetCmd := &cobra.Command{
		Use:   "reset",
		Short: "Stop all running workers and remove their containers",
		RunE:  runWorkerReset,
	}

	var reviewShell bool
	workerReviewCmd := &cobra.Command{
		Use:   "review",
		Short: "Start a reviewer that reviews open PRs and merges approved ones",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReview(reviewShell)
		},
	}
	workerReviewCmd.Flags().BoolVar(&reviewShell, "shell", false, "Open a shell instead of launching claude (for debugging)")

	workerCmd.AddCommand(workerListCmd, workerStartCmd, workerStopCmd, workerResetCmd, workerReviewCmd)
	rootCmd.AddCommand(workerCmd)

	// Top-level alias: sindri work = sindri worker start
	var workSkill string
	var workShell bool
	workCmd := &cobra.Command{
		Use:   "work [name]",
		Short: "Start a worker (alias for 'worker start')",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkerStart(args, workSkill, workShell)
		},
	}
	workCmd.Flags().StringVar(&workSkill, "skill", "", "Skill to run (e.g. td-next, td-review)")
	workCmd.Flags().BoolVar(&workShell, "shell", false, "Open a shell instead of launching claude")
	rootCmd.AddCommand(workCmd)

	rootCmd.AddCommand(newTuiCmd())
	rootCmd.AddCommand(newPrCmd())
	rootCmd.AddCommand(newReviewCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runWorkerStart(args []string, skill string, shell bool) error {
	projectRoot, err := worker.GitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}
	if err := worker.EnsureImage(projectRoot); err != nil {
		return err
	}

	var name string
	if len(args) > 0 {
		name = args[0]
	}

	if name == "" {
		var created bool
		name, created, err = worker.FindAvailable(projectRoot)
		if err != nil {
			return err
		}
		if created {
			fmt.Fprintf(os.Stderr, "🔨 %s created\n", name)
		} else {
			fmt.Fprintf(os.Stderr, "🔨 resuming %s\n", name)
		}
	}

	return worker.Start(projectRoot, name, worker.StartOpts{Skill: skill, Shell: shell})
}

func runWorkerList(cmd *cobra.Command, args []string) error {
	projectRoot, err := worker.GitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	workers := worker.List(projectRoot)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tROLE\tSTATUS\tTASK\tPR")
	fmt.Fprintln(w, "----\t----\t------\t----\t--")
	for _, wk := range workers {
		icon := "🔨"
		if wk.IsMain {
			icon = "👑"
		} else if wk.Role == "orphan" {
			icon = "⚠ "
		}
		fmt.Fprintf(w, "%s %s\t%s\t%s\t%s\t%s\n", icon, wk.Name, wk.Role, wk.Status, wk.Task, wk.PR)
	}
	w.Flush()
	return nil
}

func runWorkerReset(cmd *cobra.Command, args []string) error {
	projectRoot, err := worker.GitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}
	stopped, err := worker.Reset(projectRoot)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Stopped %d worker(s).\n", stopped)
	return nil
}

func runWorkerStop(cmd *cobra.Command, args []string) error {
	return worker.Stop(args[0])
}
