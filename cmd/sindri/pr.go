// package: main (sindri) / pr
// type:    command
// job:     the `sindri pr` subcommands (list/info/view/next/try/approve/merge/
//          reject/review), rendering PRs and tasks for the human reviewer.
// limits:  no domain logic — gate/status rules (-> issue), styling (-> render),
//          PR storage (-> ghlocal/store), task I/O (-> adapter/td).
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/flo-at/sindri/internal/action"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/flo-at/sindri/internal/issue"
	"github.com/flo-at/sindri/internal/render"
	"github.com/flo-at/sindri/internal/worker"
	"github.com/spf13/cobra"
)

func selectedPRPath() string {
	root, err := worker.GitRoot()
	if err != nil {
		return ""
	}
	return filepath.Join(root, ".git", "sindri-selected-pr")
}

func readSelectedPR() string {
	path := selectedPRPath()
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeSelectedPR(id string) {
	if path := selectedPRPath(); path != "" {
		if err := os.WriteFile(path, []byte(id+"\n"), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: write selected PR: %v\n", err)
		}
	}
}

// resolveSelectedPR returns the explicit arg or the selected PR.
func resolveSelectedPR(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	sel := readSelectedPR()
	if sel == "" {
		return "", fmt.Errorf("no PR specified and none selected — run 'sindri pr next' first")
	}
	return sel, nil
}

func newPrCmd() *cobra.Command {
	prCmd := &cobra.Command{
		Use:   "pr",
		Short: "Manage local pull requests",
	}

	var prListAll bool
	prListCmd := &cobra.Command{
		Use:   "list",
		Short: "List local PRs (open + approved by default, --all for everything)",
		RunE: func(cmd *cobra.Command, args []string) error {
			prs, err := store.List()
			if err != nil {
				return err
			}
			selected := readSelectedPR()
			rows := make([][]string, 0, len(prs))
			selectedRow := -1
			for _, pr := range prs {
				if !prListAll && pr.Status != "open" && pr.Status != "approved" {
					continue
				}
				reviews := ""
				if taskID := issue.TaskIDFromTitle(pr.Title); taskID != "" {
					if iss, err := td.Get(tdWorkDir(), taskID); err == nil {
						reviews = render.Gates(iss.Gates())
					}
				}
				if pr.ID == selected {
					selectedRow = len(rows)
				}
				rows = append(rows, []string{pr.ID, pr.Status, pr.Branch + " → " + pr.Base, reviews, pr.Title})
			}
			if len(rows) == 0 {
				fmt.Println("No PRs found.")
				return nil
			}
			dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
			selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).Padding(0, 1)
			t := table.New().
				Headers("ID", "STATUS", "BRANCH", "REVIEWS", "TITLE").
				Rows(rows...).
				BorderStyle(dim).
				StyleFunc(func(row, col int) lipgloss.Style {
					if row == table.HeaderRow {
						return lipgloss.NewStyle().Bold(true).Padding(0, 1)
					}
					if row == selectedRow {
						return selectedStyle
					}
					return lipgloss.NewStyle().Padding(0, 1)
				})
			fmt.Println(t)
			return nil
		},
	}
	prListCmd.Flags().BoolVar(&prListAll, "all", false, "Show all PRs, not just open")
	var approveAndMerge bool
	approveAndMergeCmd := &cobra.Command{
		Use:   "approve [id]",
		Short: "Approve a PR (defaults to selected, --merge to also merge)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveSelectedPR(args)
			if err != nil {
				return err
			}
			pr, err := store.Read(id)
			if err != nil {
				return err
			}
			fmt.Printf("PR:    %s\n", pr.ID)
			fmt.Printf("Title: %s\n", pr.Title)
			taskID := issue.TaskIDFromTitle(pr.Title)
			if taskID != "" {
				printTaskSummary(taskID)
			}
			if _, err := action.Approve(tdWorkDir(), id); err != nil {
				return err
			}
			fmt.Printf("Approved PR: %s\n", id)
			if !approveAndMerge {
				return nil
			}
			if !confirmHuman() {
				return fmt.Errorf("aborted")
			}
			return mergeAndReport(id)
		},
	}
	approveAndMergeCmd.Flags().BoolVar(&approveAndMerge, "merge", false, "Also merge after approving")

	prCmd.AddCommand(prListCmd,
		&cobra.Command{
			Use:   "info [id]",
			Short: "Short PR summary (defaults to selected)",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				id, err := resolveSelectedPR(args)
				if err != nil {
					return err
				}
				return printPRInfo(id)
			},
		},
		&cobra.Command{
			Use:   "view [id]",
			Short: "View a PR with full diff (defaults to selected)",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				id, err := resolveSelectedPR(args)
				if err != nil {
					return err
				}
				pr, err := store.Read(id)
				if err != nil {
					return err
				}
				fmt.Printf("PR:     %s\n", pr.ID)
				fmt.Printf("Title:  %s\n", pr.Title)
				fmt.Printf("Branch: %s → %s\n", pr.Branch, pr.Base)
				fmt.Printf("Status: %s\n", pr.Status)
				if pr.Body != "" {
					fmt.Printf("\n%s\n", pr.Body)
				}
				if pr.Diff != "" {
					fmt.Printf("\n--- diff ---\n%s\n", pr.Diff)
				}
				return nil
			},
		},
		approveAndMergeCmd,
		&cobra.Command{
			Use:   "merge [id]",
			Short: "Merge an approved PR (defaults to selected)",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				id, err := resolveSelectedPR(args)
				if err != nil {
					return err
				}
				pr, err := store.Read(id)
				if err != nil {
					return err
				}
				fmt.Printf("PR:    %s\n", pr.ID)
				fmt.Printf("Title: %s\n", pr.Title)
				taskID := issue.TaskIDFromTitle(pr.Title)
				if taskID != "" {
					printTaskSummary(taskID)
				}
				if !confirmHuman() {
					return fmt.Errorf("aborted")
				}
				return mergeAndReport(id)
			},
		},
	)

	var rejectComment string
	rejectCmd := &cobra.Command{
		Use:   "reject [id]",
		Short: "Reject a PR and its associated task (defaults to selected)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveSelectedPR(args)
			if err != nil {
				return err
			}
			comment := rejectComment
			if comment == "" {
				fmt.Print("Rejection reason: ")
				reader := bufio.NewReader(os.Stdin)
				var readErr error
				comment, readErr = reader.ReadString('\n')
				if readErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: reading input: %v\n", readErr)
				}
				comment = strings.TrimSpace(comment)
			}
			pr, err := action.Reject(tdWorkDir(), id, comment)
			if err != nil {
				return err
			}
			fmt.Printf("Rejected PR %s and returned its task to open\n", pr.ID)
			return nil
		},
	}
	rejectCmd.Flags().StringVarP(&rejectComment, "comment", "c", "", "Rejection reason")
	prCmd.AddCommand(rejectCmd)

	prCmd.AddCommand(
		&cobra.Command{
			Use:   "try [id]",
			Short: "Check out a PR in review worktree and build (defaults to selected)",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				id, err := resolveSelectedPR(args)
				if err != nil {
					return err
				}
				pr, err := store.Read(id)
				if err != nil {
					return err
				}

				projectRoot, err := worker.GitRoot()
				if err != nil {
					return err
				}

				wtPath := projectRoot + "/.worktrees/review"
				if _, err := os.Stat(wtPath); os.IsNotExist(err) {
					fmt.Fprintf(os.Stderr, "Creating review worktree...\n")
					out, err := exec.Command("git", "-C", projectRoot, "worktree", "add", wtPath, "HEAD", "--detach").CombinedOutput()
					if err != nil {
						return fmt.Errorf("worktree add failed: %s", strings.TrimSpace(string(out)))
					}
				}

				// Reset to base, then merge the PR branch
				base := pr.Base
				if base == "" || base == "HEAD" {
					base = "master"
				}
				fmt.Fprintf(os.Stderr, "Resetting to %s...\n", base)
				if out, err := exec.Command("git", "-C", wtPath, "reset", "--hard").CombinedOutput(); err != nil {
					return fmt.Errorf("git reset --hard failed: %s", strings.TrimSpace(string(out)))
				}
				if out, err := exec.Command("git", "-C", wtPath, "clean", "-fd").CombinedOutput(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: git clean failed: %s\n", strings.TrimSpace(string(out)))
				}
				if out, err := exec.Command("git", "-C", wtPath, "checkout", "--detach", base).CombinedOutput(); err != nil {
					return fmt.Errorf("checkout %s failed: %s", base, strings.TrimSpace(string(out)))
				}

				fmt.Fprintf(os.Stderr, "Merging %s...\n", pr.Branch)
				if out, err := exec.Command("git", "-C", wtPath, "merge", "--no-ff", pr.Branch, "-m", "pr-try: "+pr.ID).CombinedOutput(); err != nil {
					return fmt.Errorf("merge failed — branch %s may not exist or has conflicts:\n%s", pr.Branch, strings.TrimSpace(string(out)))
				}

				fmt.Fprintf(os.Stderr, "Building in %s...\n", wtPath)
				build := exec.Command("make")
				build.Dir = wtPath
				build.Stdout = os.Stdout
				build.Stderr = os.Stderr
				if err := build.Run(); err != nil {
					return fmt.Errorf("build failed: %w", err)
				}

				fmt.Printf("\nReady to test: %s\n", wtPath)
				fmt.Printf("Binaries: %s/bin/\n", wtPath)
				return nil
			},
		},
	)

	prCmd.AddCommand(
		&cobra.Command{
			Use:   "next",
			Short: "Select the next PR: open ones first, then approved-but-not-merged",
			RunE: func(cmd *cobra.Command, args []string) error {
				prs, err := store.List()
				if err != nil {
					return err
				}
				// Prefer open (need review), then approved (need merge)
				var fallback *store.PR
				for _, pr := range prs {
					if pr.Status == "open" {
						writeSelectedPR(pr.ID)
						fmt.Printf("Selected: %s\n\n", pr.ID)
						return printPRInfo(pr.ID)
					}
					if pr.Status == "approved" && fallback == nil {
						fallback = pr
					}
				}
				if fallback != nil {
					writeSelectedPR(fallback.ID)
					fmt.Printf("Selected (approved, ready to merge): %s\n\n", fallback.ID)
					return printPRInfo(fallback.ID)
				}
				fmt.Println("No open or approved PRs.")
				return nil
			},
		},
	)

	reviewCmd := &cobra.Command{
		Use:   "review <id>",
		Short: "Show review gate status for a PR's task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pr, err := store.Read(args[0])
			if err != nil {
				return err
			}
			taskID := issue.TaskIDFromTitle(pr.Title)
			if taskID == "" {
				return fmt.Errorf("no task ID found in PR title")
			}
			iss, err := td.Get(tdWorkDir(), taskID)
			if err != nil {
				return err
			}
			gates := iss.Gates()
			if len(gates) == 0 {
				fmt.Printf("No review gates on %s\n", taskID)
				return nil
			}
			fmt.Printf("Review gates for %s (%s):\n", args[0], taskID)
			for _, g := range gates {
				fmt.Printf("  %s\n", render.GateLabel(g))
			}
			return nil
		},
	}
	reviewCmd.AddCommand(&cobra.Command{
		Use:   "approve <pr-id> <gate>",
		Short: "Mark a review gate as approved (e.g. 'code', 'security')",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pr, err := store.Read(args[0])
			if err != nil {
				return err
			}
			taskID := issue.TaskIDFromTitle(pr.Title)
			if taskID == "" {
				return fmt.Errorf("no task ID found in PR title")
			}
			gate := args[1]
			requireLabel := "require-review-" + gate
			approveLabel := "approved-review-" + gate

			labels, err := getTaskLabels(taskID)
			if err != nil {
				return err
			}
			for _, l := range labels {
				if l == approveLabel {
					fmt.Printf("Gate '%s' already approved on %s\n", gate, taskID)
					return nil
				}
			}
			hasRequire := false
			for _, l := range labels {
				if l == requireLabel {
					hasRequire = true
					break
				}
			}
			if !hasRequire {
				labels = append(labels, requireLabel)
			}
			labels = append(labels, approveLabel)
			if err := td.SetLabels(tdWorkDir(), taskID, labels); err != nil {
				return err
			}
			fmt.Printf("Approved gate '%s' on %s (%s)\n", gate, args[0], taskID)
			return nil
		},
	})
	prCmd.AddCommand(reviewCmd)

	return prCmd
}

func confirmHuman() bool {
	fmt.Print("Agents don't approve/merge. Confirm that you are a human user (y/n): ")
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: reading input: %v\n", err)
		return false
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

func printPRInfo(id string) error {
	pr, err := store.Read(id)
	if err != nil {
		return err
	}
	fmt.Printf("%s [%s] %s → %s\n", pr.ID, pr.Status, pr.Branch, pr.Base)
	fmt.Printf("%s\n", pr.Title)
	if taskID := issue.TaskIDFromTitle(pr.Title); taskID != "" {
		fmt.Println()
		printTaskSummary(taskID)
		fmt.Println()
		if out, err := td.Show(tdWorkDir(), taskID); err == nil {
			fmt.Println(out)
		}
		if c, err := td.Comments(tdWorkDir(), taskID); err == nil && c != "" && c != "No comments" {
			fmt.Printf("\n--- Comments ---\n%s\n", c)
		}
	}
	return nil
}

func printTaskSummary(taskID string) {
	t, err := td.Get(tdWorkDir(), taskID)
	if err != nil {
		return
	}
	fmt.Printf("Task:     %s [%s] %s\n", taskID, t.Status, t.Title)
	if gates := render.Gates(t.Gates()); gates != "" {
		fmt.Printf("  %s\n", gates)
	}
}

// getTaskLabels returns a task's labels (thin wrapper over the td adapter).
func getTaskLabels(taskID string) ([]string, error) {
	t, err := td.Get(tdWorkDir(), taskID)
	if err != nil {
		return nil, err
	}
	return t.Labels, nil
}

func newRejectCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "reject <task-id>",
		Short: "Reject a task back for rework and reject its open PRs (requires -m reason)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := action.RejectTask(tdWorkDir(), args[0], reason); err != nil {
				return err
			}
			fmt.Printf("Rejected task %s and its open PRs\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&reason, "message", "m", "", "Reason for rejection (required)")
	return cmd
}

// tdWorkDir returns the project root for td commands.
// After store.Merge() does git checkout, cwd may not be the project root.
func tdWorkDir() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "."
	}
	return strings.TrimSpace(string(out))
}

// mergeAndReport merges a PR through the shared action layer (gate-checked, then
// closes the task) and prints the outcome. Human confirmation is the caller's.
func mergeAndReport(id string) error {
	merged, missing, err := action.Merge(tdWorkDir(), id)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "Review gates not met:\n")
		for _, m := range missing {
			fmt.Fprintf(os.Stderr, "  ✗ %s\n", m)
		}
		return fmt.Errorf("missing reviews: %s", strings.Join(missing, ", "))
	}
	fmt.Printf("Merged PR %s into %s\n", merged.ID, merged.Base)
	if taskID := issue.TaskIDFromTitle(merged.Title); taskID != "" {
		fmt.Printf("Closed task %s\n", taskID)
	}
	return nil
}
