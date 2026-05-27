package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/flo-at/sindri/internal/ghlocal/store"
)

var (
	createTitle string
	createBody  string
	createBase  string
	createTask  string
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

		// Rebase onto base branch before creating PR
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

		fmt.Printf("[sindri local git] PR created: %s (%s → %s)\n", pr.ID, branch, base)
		fmt.Printf("[sindri local git] Rebased onto %s.\n", base)
		fmt.Println()
		fmt.Println("╔══════════════════════════════════════════════════════════╗")
		fmt.Println("║  STOP — YOUR WORK IS DONE                              ║")
		fmt.Println("║                                                         ║")
		fmt.Println("║  1. Do NOT merge this PR — the reviewer will merge it   ║")
		fmt.Println("║  2. Do NOT approve your own PR                          ║")
		fmt.Println("║  3. Run: td handoff <task-id> --done \"what you did\"     ║")
		fmt.Println("║  4. Run: td review <task-id>                            ║")
		fmt.Println("║  5. STOP. Wait for the reviewer.                        ║")
		fmt.Println("╚══════════════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Printf("[sindri local git] Waiting for human review...\n")

		// Poll PR status until approved or rejected
		for {
			time.Sleep(5 * time.Second)
			current, err := store.Read(pr.ID)
			if err != nil {
				continue
			}
			switch current.Status {
			case "approved", "merged":
				fmt.Printf("[sindri local git] PR approved! Reviewer will handle the merge.\n")
				return nil
			case "open":
				// Still waiting
				continue
			default:
				// Rejected or unknown
				fmt.Printf("[sindri local git] PR status changed to: %s\n", current.Status)
				fmt.Printf("[sindri local git] Check td comments for reviewer feedback.\n")
				return fmt.Errorf("PR %s was not approved (status: %s)", pr.ID, current.Status)
			}
		}
	},
}

func init() {
	prCreateCmd.Flags().StringVarP(&createTitle, "title", "t", "", "PR title (defaults to branch name)")
	prCreateCmd.Flags().StringVarP(&createBody, "body", "b", "", "PR body")
	prCreateCmd.Flags().StringVar(&createBase, "base", "", "Base branch (default: GH_LOCAL_BASE or main)")
	prCreateCmd.Flags().StringVar(&createTask, "task", "", "Task ID for unique PR naming (e.g. td-8a5b6d)")
}

var taskIDPattern = regexp.MustCompile(`\(?(td-[0-9a-f]+)\)?`)

// resolveID checks for existing PRs with the same ID.
// If open/approved: returns an error. If merged: appends -2, -3, etc.
func resolveCreateID(baseID string) (string, error) {
	existing, err := store.Read(baseID)
	if err != nil {
		return baseID, nil // doesn't exist, use as-is
	}
	if existing.Status == "open" || existing.Status == "approved" {
		return "", fmt.Errorf("PR %s already exists (status: %s). Close or merge it first", baseID, existing.Status)
	}
	// Merged — find next revision
	for rev := 2; ; rev++ {
		candidate := fmt.Sprintf("%s-%d", baseID, rev)
		existing, err := store.Read(candidate)
		if err != nil {
			return candidate, nil // doesn't exist
		}
		if existing.Status == "open" || existing.Status == "approved" {
			return "", fmt.Errorf("PR %s already exists (status: %s). Close or merge it first", candidate, existing.Status)
		}
		// Also merged, try next revision
	}
}

// prIDFromTask derives a PR ID from --task flag, title, or branch name.
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
		// Fall back to simple diff if three-dot diff fails
		out, err = exec.Command("git", "diff", base, branch).Output()
		if err != nil {
			return "", err
		}
	}
	return string(out), nil
}
