// package: gh / pr_create
// type:    command
// job:     `gh pr create` plus the PR-ID derivation/revision helpers; rebases
//          and records a PR, then waits for review.
// limits:  persistence in store; prefer `gh submit`, which wraps this.
package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/spf13/cobra"
)

var (
	createTitle string
	createBody  string
	createBase  string
	createTask  string
)

var prCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a local PR from the current branch (prefer 'gh submit')",
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()
		branch, err := currentBranch()
		if err != nil {
			return err
		}
		if branch == "" {
			return fmt.Errorf("could not determine current branch")
		}

		base := createBase
		if base == "" {
			base = baseBranch()
		}

		if out, err := exec.Command("git", "rebase", base).CombinedOutput(); err != nil {
			return fmt.Errorf("rebase onto %s failed: %s", base, strings.TrimSpace(string(out)))
		}

		diff, err := gitDiff(base, branch)
		if err != nil {
			return fmt.Errorf("git diff failed: %w", err)
		}

		title := createTitle
		if title == "" {
			title = branch
		}

		baseID := prIDFromTask(createTask, title, branch)
		prID, err := resolveCreateID(baseID)
		if err != nil {
			return err
		}

		pr := &store.PR{
			ID:        prID,
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

		fmt.Printf("PR created: %s (%s → %s)\n", pr.ID, branch, base)
		fmt.Println()
		fmt.Println("╔══════════════════════════════════════════════════════════╗")
		fmt.Println("║  Use 'gh submit' instead — it handles handoff + review  ║")
		fmt.Println("╚══════════════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Println("Submitted. Run 'gh done' then 'gh issue next' for the next task.")
		return nil
	},
}

func init() {
	prCreateCmd.Flags().StringVarP(&createTitle, "title", "t", "", "PR title")
	prCreateCmd.Flags().StringVarP(&createBody, "body", "b", "", "PR body")
	prCreateCmd.Flags().StringVar(&createBase, "base", "", "Base branch (default: auto-detect)")
	prCreateCmd.Flags().StringVar(&createTask, "task", "", "Task ID for PR naming")
}

var taskIDPattern = regexp.MustCompile(`\(?(td-[0-9a-f]+)\)?`)

func resolveCreateID(baseID string) (string, error) {
	existing, err := store.Read(baseID)
	if err != nil {
		return baseID, nil
	}
	if existing.Status == "open" || existing.Status == "approved" {
		return "", fmt.Errorf("PR %s already exists (status: %s). Close or merge it first", baseID, existing.Status)
	}
	for rev := 2; ; rev++ {
		candidate := fmt.Sprintf("%s-%d", baseID, rev)
		existing, err := store.Read(candidate)
		if err != nil {
			return candidate, nil
		}
		if existing.Status == "open" || existing.Status == "approved" {
			return "", fmt.Errorf("PR %s already exists (status: %s)", candidate, existing.Status)
		}
	}
}

func prIDFromTask(task, title, branch string) string {
	if task != "" {
		return "pr-" + task
	}
	if m := taskIDPattern.FindStringSubmatch(title); len(m) > 1 {
		return "pr-" + m[1]
	}
	return "pr-" + branch
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
		out, err = exec.Command("git", "diff", base, branch).Output()
		if err != nil {
			return "", err
		}
	}
	return string(out), nil
}

// currentTaskID returns the task ID from the current branch name, or empty if not on a task branch.
func currentTaskID() string {
	branch, err := currentBranch()
	if err != nil {
		return ""
	}
	if taskIDPattern.MatchString(branch) {
		return branch
	}
	return ""
}
