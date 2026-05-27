package cmd

import (
	"encoding/json"
	"fmt"
	"os"
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
		// Accept base branch or detached HEAD (from gh done)
		if branch != base && branch != "HEAD" {
			return fmt.Errorf("you are on branch %q — run 'gh done' first to return to %s", branch, base)
		}

		// Clean up any orphaned in_progress tasks from previous runs
		if listOut, err := td("query", "status:in_progress"); err == nil {
			for _, line := range strings.Split(listOut, "\n") {
				f := strings.Fields(strings.TrimSpace(line))
				if len(f) > 0 && strings.HasPrefix(f[0], "td-") {
					td("unstart", f[0])
				}
			}
		}

		out, err := td("next")
		if err != nil {
			fmt.Println("No tasks available.")
			return nil
		}

		// Parse td next output: "td-abc123  [P0]  Title  type  [status]"
		fields := strings.Fields(out)
		if len(fields) < 1 || !strings.HasPrefix(fields[0], "td-") {
			fmt.Println("No tasks available.")
			return nil
		}

		var task struct {
			ID    string
			Title string
		}
		task.ID = fields[0]
		// Extract title: everything between priority and type/status fields
		var titleParts []string
		for _, f := range fields[2:] {
			if f == "task" || f == "bug" || f == "feature" || f == "chore" || f == "epic" {
				break
			}
			if strings.HasPrefix(f, "[") && strings.HasSuffix(f, "]") {
				continue
			}
			titleParts = append(titleParts, f)
		}
		task.Title = strings.Join(titleParts, " ")

		if out, err := td("start", task.ID); err != nil {
			return fmt.Errorf("td start failed: %s", out)
		}
		fmt.Printf("Started task: %s %s\n\n", task.ID, task.Title)

		// Update statusline
		_ = os.WriteFile("/tmp/claude-status", []byte(task.ID+": "+task.Title), 0644)

		// Create per-task branch from base (works from detached HEAD or base branch)
		if out, err := exec.Command("git", "checkout", "-b", task.ID, base).CombinedOutput(); err != nil {
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
