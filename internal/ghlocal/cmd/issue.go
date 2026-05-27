package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/spf13/cobra"
)

var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Manage tasks (issues)",
}

var issueListState string

var issueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()

		tdArgs := []string{"list", "--json", "--limit", "50"}
		if issueListState != "" {
			tdArgs = append(tdArgs, "--status", issueListState)
		}
		out, err := td(tdArgs...)
		if err != nil {
			return fmt.Errorf("td list failed: %s", out)
		}
		var tasks []struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Status   string `json:"status"`
			Priority string `json:"priority"`
		}
		if err := json.Unmarshal([]byte(out), &tasks); err != nil {
			return fmt.Errorf("parse failed: %w", err)
		}
		if len(tasks) == 0 {
			fmt.Println("No tasks found.")
			return nil
		}
		for _, t := range tasks {
			fmt.Printf("%-12s  %-4s  %-12s  %s\n", t.ID, t.Priority, t.Status, t.Title)
		}
		return nil
	},
}

var issueViewCmd = &cobra.Command{
	Use:   "view [task-id]",
	Short: "View task details including comments, branch, and PR status",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()
		taskID := resolveTaskID(args)
		if taskID == "" {
			return fmt.Errorf("no task ID given and not on a task branch")
		}

		if err := printTaskDetails(taskID); err != nil {
			return err
		}

		prs, err := store.List()
		if err == nil {
			for _, pr := range prs {
				if m := taskIDPattern.FindStringSubmatch(pr.Title); len(m) > 1 && m[1] == taskID {
					fmt.Printf("\nPR: %s [%s] %s → %s\n", pr.ID, pr.Status, pr.Branch, pr.Base)
				}
			}
		}

		if branch, err := currentBranch(); err == nil {
			fmt.Printf("\nCurrent branch: %s\n", branch)
		}
		return nil
	},
}

var issueCommentBody string

var issueCommentCmd = &cobra.Command{
	Use:   "comment [task-id]",
	Short: "Add a comment to a task (defaults to current task from branch)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()
		taskID := resolveTaskID(args)
		if taskID == "" {
			return fmt.Errorf("no task ID given and not on a task branch")
		}

		if issueCommentBody == "" {
			return fmt.Errorf("--body / -b is required")
		}

		out, err := td("comment", taskID, issueCommentBody)
		if err != nil {
			return fmt.Errorf("td comment failed: %s", out)
		}
		fmt.Println(out)
		return nil
	},
}

var issueNextCmd = &cobra.Command{
	Use:   "next",
	Short: "Pick up the next task: claim it, create a branch, print details",
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()

		branch, _ := currentBranch()
		base := baseBranch()
		if branch != base {
			return fmt.Errorf("you are on branch %q — run 'gh done' first to return to %s", branch, base)
		}

		out, err := td("next", "--json")
		if err != nil {
			fmt.Println("No tasks available.")
			return nil
		}

		var task struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Type     string `json:"type"`
			Priority string `json:"priority"`
		}
		if err := json.Unmarshal([]byte(out), &task); err != nil {
			return fmt.Errorf("failed to parse task: %w\n%s", err, out)
		}

		if out, err := td("start", task.ID); err != nil {
			return fmt.Errorf("td start failed: %s", out)
		}
		fmt.Printf("Started task: %s %s\n\n", task.ID, task.Title)

		// Rebase onto base branch
		if out, err := exec.Command("git", "rebase", base).CombinedOutput(); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: rebase failed: %s\n", strings.TrimSpace(string(out)))
		}

		// Create per-task branch
		if out, err := exec.Command("git", "checkout", "-b", task.ID).CombinedOutput(); err != nil {
			return fmt.Errorf("failed to create branch %s: %s", task.ID, strings.TrimSpace(string(out)))
		}
		fmt.Printf("Branch: %s (from %s)\n\n", task.ID, base)

		return printTaskDetails(task.ID)
	},
}

// resolveTaskID returns task ID from args or from current branch name.
func resolveTaskID(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return currentTaskID()
}

func printTaskDetails(taskID string) error {
	out, err := td("show", taskID)
	if err != nil {
		return fmt.Errorf("td show failed: %s", out)
	}
	fmt.Println(out)

	comments, err := td("comments", taskID)
	if err == nil && comments != "" {
		fmt.Printf("\n--- Comments ---\n%s\n", comments)
	}
	return nil
}

func init() {
	issueListCmd.Flags().StringVar(&issueListState, "state", "", "Filter by state (open, closed, etc.)")
	issueCommentCmd.Flags().StringVarP(&issueCommentBody, "body", "b", "", "Comment text")

	issueCmd.AddCommand(issueListCmd)
	issueCmd.AddCommand(issueViewCmd)
	issueCmd.AddCommand(issueCommentCmd)
	issueCmd.AddCommand(issueNextCmd)
	rootCmd.AddCommand(issueCmd)
}
