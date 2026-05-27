package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/flo-at/sindri/internal/ghlocal/store"
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
		_ = os.WriteFile(path, []byte(id+"\n"), 0644)
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
		Short: "List local PRs (open only by default, --all for everything)",
		RunE: func(cmd *cobra.Command, args []string) error {
			prs, err := store.List()
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(prs))
			for _, pr := range prs {
				if !prListAll && pr.Status != "open" {
					continue
				}
				rows = append(rows, []string{pr.ID, pr.Status, pr.Branch + " → " + pr.Base, pr.Title})
			}
			if len(rows) == 0 {
				fmt.Println("No PRs found.")
				return nil
			}
			dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
			t := table.New().
				Headers("ID", "STATUS", "BRANCH", "TITLE").
				Rows(rows...).
				BorderStyle(dim).
				StyleFunc(func(row, col int) lipgloss.Style {
					if row == table.HeaderRow {
						return lipgloss.NewStyle().Bold(true).Padding(0, 1)
					}
					return lipgloss.NewStyle().Padding(0, 1)
				})
			fmt.Println(t)
			return nil
		},
	}
	prListCmd.Flags().BoolVar(&prListAll, "all", false, "Show all PRs, not just open")
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
				pr, err := store.Read(id)
				if err != nil {
					return err
				}
				fmt.Printf("%s [%s] %s → %s\n", pr.ID, pr.Status, pr.Branch, pr.Base)
				fmt.Printf("%s\n", pr.Title)
				if taskID := extractTaskID(pr.Title); taskID != "" {
					fmt.Println()
					printTaskSummary(taskID)
					fmt.Println()
					tdShow := exec.Command("td", "show", taskID)
					tdShow.Dir = tdWorkDir()
					if out, err := tdShow.Output(); err == nil {
						fmt.Println(strings.TrimSpace(string(out)))
					}
					tdComments := exec.Command("td", "comments", taskID)
					tdComments.Dir = tdWorkDir()
					if out, err := tdComments.Output(); err == nil {
						c := strings.TrimSpace(string(out))
						if c != "" && c != "No comments" {
							fmt.Printf("\n--- Comments ---\n%s\n", c)
						}
					}
				}
				return nil
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
		&cobra.Command{
			Use:   "approve [id]",
			Short: "Approve a PR (defaults to selected)",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				id, err := resolveSelectedPR(args)
				if err != nil {
					return err
				}
				pr, err := store.Approve(id)
				if err != nil {
					return err
				}
				fmt.Printf("Approved PR: %s\n", pr.ID)
				return nil
			},
		},
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
				taskID := extractTaskID(pr.Title)
				if taskID != "" {
					if missing, err := checkReviewGates(taskID); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: could not check review gates: %v\n", err)
					} else if len(missing) > 0 {
						fmt.Fprintf(os.Stderr, "Review gates not met for %s:\n", taskID)
						for _, m := range missing {
							fmt.Fprintf(os.Stderr, "  ✗ %s\n", m)
						}
						return fmt.Errorf("missing reviews: %s", strings.Join(missing, ", "))
					}
				}
				pr, err = store.Merge(id)
				if err != nil {
					return err
				}
				fmt.Printf("Merged PR %s into %s\n", pr.ID, pr.Base)
				if taskID != "" {
					tdClose := exec.Command("td", "close", taskID, "--self-close-exception", "PR merged")
					tdClose.Dir = tdWorkDir()
					if out, err := tdClose.CombinedOutput(); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: td close %s failed: %s\n", taskID, strings.TrimSpace(string(out)))
					} else {
						fmt.Printf("Closed task %s\n", taskID)
					}
				}
				return nil
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
			pr, err := store.Read(id)
			if err != nil {
				return err
			}

			comment := rejectComment
			if comment == "" {
				fmt.Print("Rejection reason: ")
				reader := bufio.NewReader(os.Stdin)
				comment, _ = reader.ReadString('\n')
				comment = strings.TrimSpace(comment)
			}
			if comment == "" {
				return fmt.Errorf("rejection reason is required")
			}

			pr.Status = "rejected"
			if err := store.Write(pr); err != nil {
				return err
			}
			fmt.Printf("Rejected PR %s\n", pr.ID)

			if taskID := extractTaskID(pr.Title); taskID != "" {
				tdComment := exec.Command("td", "comment", taskID, comment)
				tdComment.Dir = tdWorkDir()
				if out, err := tdComment.CombinedOutput(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: td comment failed: %s\n", strings.TrimSpace(string(out)))
				}

				tdCmd := exec.Command("td", "reject", taskID)
				tdCmd.Dir = tdWorkDir()
				if out, err := tdCmd.CombinedOutput(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: td reject %s failed: %s\n", taskID, strings.TrimSpace(string(out)))
				} else {
					fmt.Printf("Rejected task %s\n", taskID)
				}
			}
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

				fmt.Fprintf(os.Stderr, "Checking out %s...\n", pr.Branch)
				if out, err := exec.Command("git", "-C", wtPath, "checkout", "--detach", pr.Branch).CombinedOutput(); err != nil {
					return fmt.Errorf("checkout failed: %s", strings.TrimSpace(string(out)))
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
			Short: "Select the next open PR for review",
			RunE: func(cmd *cobra.Command, args []string) error {
				prs, err := store.List()
				if err != nil {
					return err
				}
				for _, pr := range prs {
					if pr.Status == "open" {
						writeSelectedPR(pr.ID)
						fmt.Printf("Selected: %s\n", pr.ID)
						fmt.Printf("PR:       %s\n", pr.Title)
						fmt.Printf("Branch:   %s → %s\n", pr.Branch, pr.Base)
						if taskID := extractTaskID(pr.Title); taskID != "" {
							printTaskSummary(taskID)
						}
						return nil
					}
				}
				fmt.Println("No open PRs.")
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
			taskID := extractTaskID(pr.Title)
			if taskID == "" {
				return fmt.Errorf("no task ID found in PR title")
			}
			labels, err := getTaskLabels(taskID)
			if err != nil {
				return err
			}
			approved := make(map[string]bool)
			var required []string
			for _, l := range labels {
				if strings.HasPrefix(l, "require-review-") {
					required = append(required, strings.TrimPrefix(l, "require-review-"))
				}
				if strings.HasPrefix(l, "approved-review-") {
					approved[strings.TrimPrefix(l, "approved-review-")] = true
				}
			}
			if len(required) == 0 {
				fmt.Printf("No review gates on %s\n", taskID)
				return nil
			}
			fmt.Printf("Review gates for %s (%s):\n", args[0], taskID)
			for _, r := range required {
				if approved[r] {
					fmt.Printf("  ☑ %s\n", r)
				} else {
					fmt.Printf("  ☐ %s\n", r)
				}
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
			taskID := extractTaskID(pr.Title)
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
			labelsStr := strings.Join(labels, ",")
			tdCmd := exec.Command("td", "update", taskID, "--labels", labelsStr)
			tdCmd.Dir = tdWorkDir()
			if out, err := tdCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("td update failed: %s", strings.TrimSpace(string(out)))
			}
			fmt.Printf("Approved gate '%s' on %s (%s)\n", gate, args[0], taskID)
			return nil
		},
	})
	prCmd.AddCommand(reviewCmd)

	return prCmd
}

func printTaskSummary(taskID string) {
	tdCmd := exec.Command("td", "show", taskID, "--json")
	tdCmd.Dir = tdWorkDir()
	if out, err := tdCmd.Output(); err == nil {
		var task struct {
			Title  string   `json:"title"`
			Status string   `json:"status"`
			Labels []string `json:"labels"`
		}
		if json.Unmarshal(out, &task) == nil {
			fmt.Printf("Task:     %s [%s] %s\n", taskID, task.Status, task.Title)
			approved := make(map[string]bool)
			for _, l := range task.Labels {
				if strings.HasPrefix(l, "approved-review-") {
					approved[strings.TrimPrefix(l, "approved-review-")] = true
				}
			}
			for _, l := range task.Labels {
				if strings.HasPrefix(l, "require-review-") {
					gate := strings.TrimPrefix(l, "require-review-")
					if approved[gate] {
						fmt.Printf("  ☑ %s\n", gate)
					} else {
						fmt.Printf("  ☐ %s\n", gate)
					}
				}
			}
		}
	}
}

func getTaskLabels(taskID string) ([]string, error) {
	tdCmd := exec.Command("td", "show", taskID, "--json")
	tdCmd.Dir = tdWorkDir()
	out, err := tdCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("td show %s: %w", taskID, err)
	}
	var task struct {
		Labels []string `json:"labels"`
	}
	if err := json.Unmarshal(out, &task); err != nil {
		return nil, err
	}
	if task.Labels == nil {
		task.Labels = []string{}
	}
	return task.Labels, nil
}

// rejectPRsForTask finds and rejects all open PRs for a given task ID.
func rejectPRsForTask(taskID string) {
	prs, err := store.List()
	if err != nil {
		return
	}
	for _, pr := range prs {
		if pr.Status != "open" && pr.Status != "approved" {
			continue
		}
		if extractTaskID(pr.Title) == taskID {
			pr.Status = "rejected"
			if err := store.Write(pr); err == nil {
				fmt.Printf("Rejected PR %s\n", pr.ID)
			}
		}
	}
}

func newRejectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reject <task-id>",
		Short: "Reject a task and close its open PRs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := args[0]
			tdCmd := exec.Command("td", "reject", taskID)
			tdCmd.Dir = tdWorkDir()
			if out, err := tdCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("td reject %s failed: %s", taskID, strings.TrimSpace(string(out)))
			}
			fmt.Printf("Rejected task %s\n", taskID)
			rejectPRsForTask(taskID)
			return nil
		},
	}
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

var prTaskIDPattern = regexp.MustCompile(`\(?(td-[0-9a-f]+)\)?`)

// checkReviewGates reads a task's labels and checks that every
// require-review-X has a matching approved-review-X.
// Returns the list of missing review names (empty = all gates pass).
func checkReviewGates(taskID string) ([]string, error) {
	tdCmd := exec.Command("td", "show", taskID, "--json")
	tdCmd.Dir = tdWorkDir()
	out, err := tdCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("td show %s: %w", taskID, err)
	}
	var task struct {
		Labels []string `json:"labels"`
	}
	if err := json.Unmarshal(out, &task); err != nil {
		return nil, err
	}

	approved := make(map[string]bool)
	var required []string
	for _, label := range task.Labels {
		if strings.HasPrefix(label, "require-review-") {
			required = append(required, strings.TrimPrefix(label, "require-"))
		}
		if strings.HasPrefix(label, "approved-review-") {
			approved[strings.TrimPrefix(label, "approved-")] = true
		}
	}

	var missing []string
	for _, r := range required {
		if !approved[r] {
			missing = append(missing, r)
		}
	}
	return missing, nil
}

func extractTaskID(title string) string {
	if m := prTaskIDPattern.FindStringSubmatch(title); len(m) > 1 {
		return m[1]
	}
	return ""
}
