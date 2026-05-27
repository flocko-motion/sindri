package cmd

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/spf13/cobra"
)

var (
	submitTitle string
	submitBody  string
	submitDone  string
)

var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit your work: rebase, create PR, handoff, and wait for review",
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()

		branch, err := currentBranch()
		if err != nil {
			return err
		}

		base := baseBranch()
		if branch == base {
			return fmt.Errorf("you are on %s — nothing to submit. Run 'gh next' first", base)
		}

		taskID := branch
		if !taskIDPattern.MatchString(taskID) {
			return fmt.Errorf("branch %q doesn't look like a task ID — expected td-xxx", branch)
		}

		if submitTitle == "" {
			return fmt.Errorf("--title is required")
		}

		// Check for uncommitted changes
		if out, err := exec.Command("git", "status", "--porcelain").Output(); err == nil && len(out) > 0 {
			return fmt.Errorf("uncommitted changes — commit or stash first:\n%s", strings.TrimSpace(string(out)))
		}

		// Rebase onto base
		if out, err := exec.Command("git", "rebase", base).CombinedOutput(); err != nil {
			return fmt.Errorf("rebase onto %s failed: %s", base, strings.TrimSpace(string(out)))
		}

		// Create PR
		diff, err := gitDiff(base, branch)
		if err != nil {
			return fmt.Errorf("git diff failed: %w", err)
		}

		baseID := "pr-" + taskID
		prID, err := resolveCreateID(baseID)
		if err != nil {
			return err
		}

		pr := &store.PR{
			ID:        prID,
			Branch:    branch,
			Base:      base,
			Status:    "open",
			Title:     submitTitle,
			Body:      submitBody,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
			Diff:      diff,
		}
		if err := store.Write(pr); err != nil {
			return err
		}
		fmt.Printf("PR created: %s (%s → %s)\n", pr.ID, branch, base)

		// td handoff
		doneMsg := submitDone
		if doneMsg == "" {
			doneMsg = submitTitle
		}
		if out, err := td("handoff", taskID, "--done", doneMsg); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: td handoff failed: %s\n", out)
		}

		// td review
		if out, err := td("review", taskID); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: td review failed: %s\n", out)
		}

		fmt.Println()
		fmt.Println("╔══════════════════════════════════════════════════════════╗")
		fmt.Println("║  Submitted for review.                                  ║")
		fmt.Println("║  Run 'gh done' then 'gh issue next' for the next task.  ║")
		fmt.Println("╚══════════════════════════════════════════════════════════╝")
		return nil
	},
}

func init() {
	submitCmd.Flags().StringVarP(&submitTitle, "title", "t", "", "PR title (required, use conventional commits)")
	submitCmd.Flags().StringVarP(&submitBody, "body", "b", "", "PR body")
	submitCmd.Flags().StringVar(&submitDone, "done", "", "Handoff summary (defaults to title)")
	rootCmd.AddCommand(submitCmd)
}
