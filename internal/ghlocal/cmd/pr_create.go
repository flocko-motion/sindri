package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/flo-at/sindri/internal/ghlocal/store"
)

var (
	createTitle string
	createBody  string
	createBase  string
)

var prCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a local PR from the current branch",
	RunE: func(cmd *cobra.Command, args []string) error {
		branch, err := currentBranch()
		if err != nil {
			return err
		}
		if branch == "" {
			return fmt.Errorf("could not determine current branch")
		}

		base := createBase
		if base == "" {
			base = os.Getenv("GH_LOCAL_BASE")
		}
		if base == "" {
			// Fall back to the HEAD of /repo (main worktree mount inside container)
			if out, err := exec.Command("git", "-C", "/repo", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
				base = strings.TrimSpace(string(out))
			}
		}
		if base == "" {
			return fmt.Errorf("could not determine base branch: set GH_LOCAL_BASE or --base")
		}

		diff, err := gitDiff(base, branch)
		if err != nil {
			return fmt.Errorf("git diff failed: %w", err)
		}

		title := createTitle
		if title == "" {
			title = branch
		}

		pr := &store.PR{
			ID:        "pr-" + branch,
			Branch:    branch,
			Base:      base,
			Status:    "open",
			Title:     title,
			Body:      createBody,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
			Diff:      diff,
		}

		if err := store.Write(pr); err != nil {
			return err
		}

		fmt.Printf("Created PR: %s (branch: %s → %s)\n", pr.ID, branch, base)
		return nil
	},
}

func init() {
	prCreateCmd.Flags().StringVarP(&createTitle, "title", "t", "", "PR title (defaults to branch name)")
	prCreateCmd.Flags().StringVarP(&createBody, "body", "b", "", "PR body")
	prCreateCmd.Flags().StringVar(&createBase, "base", "", "Base branch (default: GH_LOCAL_BASE or main)")
}

func currentBranch() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitDiff(base, branch string) (string, error) {
	out, err := exec.Command("git", "diff", base+"..."+branch).Output()
	if err != nil {
		// Fall back to simple diff if three-dot diff fails
		out, err = exec.Command("git", "diff", base, branch).Output()
		if err != nil {
			return "", err
		}
	}
	return string(out), nil
}
