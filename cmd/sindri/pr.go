package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/spf13/cobra"
)

func newPrCmd() *cobra.Command {
	prCmd := &cobra.Command{
		Use:   "pr",
		Short: "Manage local pull requests",
	}

	prCmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List all local PRs",
			RunE: func(cmd *cobra.Command, args []string) error {
				prs, err := store.List()
				if err != nil {
					return err
				}
				if len(prs) == 0 {
					fmt.Println("No PRs found.")
					return nil
				}
				rows := make([][]string, 0, len(prs))
				for _, pr := range prs {
					rows = append(rows, []string{pr.ID, pr.Status, pr.Branch + " → " + pr.Base, pr.Title})
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
		},
		&cobra.Command{
			Use:   "view <id>",
			Short: "View a PR",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				pr, err := store.Read(args[0])
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
			Use:   "approve <id>",
			Short: "Approve a PR",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				pr, err := store.Approve(args[0])
				if err != nil {
					return err
				}
				fmt.Printf("Approved PR: %s\n", pr.ID)
				return nil
			},
		},
		&cobra.Command{
			Use:   "merge <id>",
			Short: "Merge an approved PR",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				pr, err := store.Read(args[0])
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
				pr, err = store.Merge(args[0])
				if err != nil {
					return err
				}
				fmt.Printf("Merged PR %s into %s\n", pr.ID, pr.Base)
				if taskID != "" {
					if out, err := exec.Command("td", "close", taskID, "--self-close-exception", "PR merged").CombinedOutput(); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: td close %s failed: %s\n", taskID, out)
					} else {
						fmt.Printf("Closed task %s\n", taskID)
					}
				}
				return nil
			},
		},
	)

	prCmd.AddCommand(
		&cobra.Command{
			Use:   "reject <id>",
			Short: "Reject a PR and its associated task",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				pr, err := store.Read(args[0])
				if err != nil {
					return err
				}
				pr.Status = "rejected"
				if err := store.Write(pr); err != nil {
					return err
				}
				fmt.Printf("Rejected PR %s\n", pr.ID)
				if taskID := extractTaskID(pr.Title); taskID != "" {
					if out, err := exec.Command("td", "reject", taskID).CombinedOutput(); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: td reject %s failed: %s\n", taskID, out)
					} else {
						fmt.Printf("Rejected task %s\n", taskID)
					}
				}
				return nil
			},
		},
	)

	return prCmd
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
			if out, err := exec.Command("td", "reject", taskID).CombinedOutput(); err != nil {
				return fmt.Errorf("td reject %s failed: %s", taskID, strings.TrimSpace(string(out)))
			}
			fmt.Printf("Rejected task %s\n", taskID)
			rejectPRsForTask(taskID)
			return nil
		},
	}
}

var prTaskIDPattern = regexp.MustCompile(`\(?(td-[0-9a-f]+)\)?`)

// checkReviewGates reads a task's labels and checks that every
// require-review-X has a matching approved-review-X.
// Returns the list of missing review names (empty = all gates pass).
func checkReviewGates(taskID string) ([]string, error) {
	out, err := exec.Command("td", "show", taskID, "--json").Output()
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
			required = append(required, strings.TrimPrefix(label, "require-review-"))
		}
		if strings.HasPrefix(label, "approved-review-") {
			approved[strings.TrimPrefix(label, "approved-review-")] = true
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
