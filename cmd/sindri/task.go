package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/flo-at/sindri/internal/worker"
	"github.com/spf13/cobra"
)

type cliTask struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Type      string   `json:"type"`
	Priority  string   `json:"priority"`
	Labels    []string `json:"labels"`
	UpdatedAt string   `json:"updated_at"`
}

func newTaskCmd() *cobra.Command {
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks",
	}

	var commentMsg string
	commentCmd := &cobra.Command{
		Use:   "comment <id>",
		Short: "Add a comment to a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			msg := commentMsg
			if msg == "" {
				fmt.Print("Comment: ")
				reader := bufio.NewReader(os.Stdin)
				var readErr error
				msg, readErr = reader.ReadString('\n')
				if readErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: reading input: %v\n", readErr)
				}
				msg = strings.TrimSpace(msg)
			}
			if msg == "" {
				return fmt.Errorf("comment is required")
			}
			out, err := exec.Command("td", "comment", args[0], msg).CombinedOutput()
			if err != nil {
				return fmt.Errorf("td comment failed: %s", strings.TrimSpace(string(out)))
			}
			fmt.Println(strings.TrimSpace(string(out)))
			return nil
		},
	}
	commentCmd.Flags().StringVarP(&commentMsg, "message", "m", "", "Comment text")

	taskCmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List all tasks with PRs and workers",
			RunE:  runTaskList,
		},
		&cobra.Command{
			Use:   "view <id>",
			Short: "View task details",
			Args:  cobra.ExactArgs(1),
			RunE:  runTaskView,
		},
		commentCmd,
	)

	return taskCmd
}

func runTaskList(cmd *cobra.Command, args []string) error {
	projectRoot, err := worker.GitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	tasks, err := listAllTasks(projectRoot)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		fmt.Println("No tasks found.")
		return nil
	}

	workers := worker.List(projectRoot)
	workersByTask := make(map[string]string)
	for _, wk := range workers {
		if wk.Task != "" {
			parts := strings.Fields(wk.Task)
			if len(parts) > 0 {
				workersByTask[parts[0]] = wk.Name
			}
		}
	}

	prs, _ := store.ListFor(projectRoot)
	prByTask := make(map[string][]*store.PR)
	for _, pr := range prs {
		if m := prTaskIDPattern.FindStringSubmatch(pr.Title); len(m) > 1 {
			prByTask[m[1]] = append(prByTask[m[1]], pr)
		}
	}

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	rows := make([][]string, 0)
	for _, t := range tasks {
		status := t.Status
		if w, ok := workersByTask[t.ID]; ok && t.Status == "in_progress" {
			status = "🔨 " + w
		}
		updated := ""
		if ts, err := time.Parse(time.RFC3339Nano, t.UpdatedAt); err == nil {
			updated = ts.Local().Format("06-01-02 15:04")
		}
		rows = append(rows, []string{t.Priority, updated, status, t.Title})

		for _, pr := range prByTask[t.ID] {
			rows = append(rows, []string{"", "", "", fmt.Sprintf("  └ %s [%s]", pr.ID, pr.Status)})
		}

		if gates := cliGateStatus(t.Labels); gates != "" {
			rows = append(rows, []string{"", "", "", "  " + gates})
		}
	}

	tbl := table.New().
		Headers("PRIO", "UPDATED", "STATUS", "TITLE").
		Rows(rows...).
		BorderStyle(dim).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true).Padding(0, 1)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})
	fmt.Println(tbl)
	return nil
}

func runTaskView(cmd *cobra.Command, args []string) error {
	projectRoot, err := worker.GitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	taskID := args[0]
	out, err := exec.Command("td", "-w", projectRoot, "show", taskID).Output()
	if err != nil {
		return fmt.Errorf("task %s not found", taskID)
	}
	fmt.Println(strings.TrimSpace(string(out)))

	prs, _ := store.ListFor(projectRoot)
	fmt.Println()
	for _, pr := range prs {
		if m := prTaskIDPattern.FindStringSubmatch(pr.Title); len(m) > 1 && m[1] == taskID {
			fmt.Printf("PR: %s [%s] %s → %s\n", pr.ID, pr.Status, pr.Branch, pr.Base)
		}
	}

	return nil
}

func listAllTasks(projectRoot string) ([]cliTask, error) {
	out, err := exec.Command("td", "-w", projectRoot, "list", "--json", "--limit", "100", "--all").Output()
	if err != nil {
		return nil, fmt.Errorf("td list failed: %w", err)
	}
	var tasks []cliTask
	if err := json.Unmarshal(out, &tasks); err != nil {
		return nil, err
	}

	var open, active, closed []cliTask
	for _, t := range tasks {
		switch t.Status {
		case "open":
			open = append(open, t)
		case "in_progress", "in_review":
			active = append(active, t)
		default:
			closed = append(closed, t)
		}
	}
	byUpdated := func(items []cliTask) func(i, j int) bool {
		return func(i, j int) bool {
			ti, _ := time.Parse(time.RFC3339Nano, items[i].UpdatedAt)
			tj, _ := time.Parse(time.RFC3339Nano, items[j].UpdatedAt)
			return ti.After(tj)
		}
	}
	sort.Slice(active, byUpdated(active))
	sort.Slice(closed, byUpdated(closed))

	result := make([]cliTask, 0, len(tasks))
	result = append(result, open...)
	result = append(result, active...)
	result = append(result, closed...)
	return result, nil
}

func cliGateStatus(labels []string) string {
	approved := make(map[string]bool)
	var required []string
	for _, l := range labels {
		if strings.HasPrefix(l, "require-review-") {
			required = append(required, strings.TrimPrefix(l, "require-"))
		}
		if strings.HasPrefix(l, "approved-review-") {
			approved[strings.TrimPrefix(l, "approved-")] = true
		}
	}
	if len(required) == 0 {
		return ""
	}
	var parts []string
	for _, r := range required {
		display := strings.ReplaceAll(r, "-", " ")
		if approved[r] {
			parts = append(parts, "☑ "+display)
		} else {
			parts = append(parts, "☐ "+display)
		}
	}
	return strings.Join(parts, "  ")
}
