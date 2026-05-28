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
	"github.com/flo-at/sindri/internal/openspec"
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

	var newType, newPrio, newBody, newSpec string
	var newReview bool
	newCmd := &cobra.Command{
		Use:   "new <title>",
		Short: "Create a task, optionally linked to an openspec change (--spec)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tdArgs := []string{"create", args[0], "-t", newType, "-p", newPrio}
			if newBody != "" {
				tdArgs = append(tdArgs, "-d", newBody)
			}
			var labels []string
			if newReview {
				labels = append(labels, "require-review-code")
			}
			if newSpec != "" {
				labels = append(labels, "spec:"+newSpec)
			}
			if len(labels) > 0 {
				tdArgs = append(tdArgs, "--labels", strings.Join(labels, ","))
			}
			out, err := exec.Command("td", tdArgs...).CombinedOutput()
			if err != nil {
				return fmt.Errorf("td create failed: %s", strings.TrimSpace(string(out)))
			}
			fmt.Println(strings.TrimSpace(string(out)))
			if newSpec != "" {
				fmt.Printf("Linked to spec: %s\n", newSpec)
			}
			return nil
		},
	}
	newCmd.Flags().StringVarP(&newType, "type", "t", "task", "Task type (task, bug, feature, chore)")
	newCmd.Flags().StringVarP(&newPrio, "priority", "p", "P2", "Priority (P0-P4)")
	newCmd.Flags().StringVarP(&newBody, "body", "d", "", "Task description")
	newCmd.Flags().StringVar(&newSpec, "spec", "", "Link to an openspec change by name")
	newCmd.Flags().BoolVar(&newReview, "review", true, "Add require-review-code gate")

	var taskListAll, taskListClosed, taskListOpen bool
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks (hides closed by default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskList(cmd, args, taskListAll, taskListOpen, taskListClosed)
		},
	}
	listCmd.Flags().BoolVar(&taskListAll, "all", false, "Show all tasks including closed")
	listCmd.Flags().BoolVar(&taskListClosed, "closed", false, "Show closed tasks")
	listCmd.Flags().BoolVar(&taskListOpen, "open", false, "Show only open tasks")

	taskCmd.AddCommand(
		listCmd,
		newCmd,
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

func runTaskList(cmd *cobra.Command, args []string, showAll, showOpen, showClosed bool) error {
	projectRoot, err := worker.GitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	tasks, err := listAllTasks(projectRoot)
	if err != nil {
		return err
	}

	if !showAll {
		filtered := tasks[:0]
		for _, t := range tasks {
			isClosed := t.Status == "closed" || t.Status == "approved" || t.Status == "merged"
			if showOpen && showClosed {
				filtered = append(filtered, t)
			} else if showOpen {
				if !isClosed {
					filtered = append(filtered, t)
				}
			} else if showClosed {
				if isClosed {
					filtered = append(filtered, t)
				}
			} else {
				if !isClosed {
					filtered = append(filtered, t)
				}
			}
		}
		tasks = filtered
	}

	if len(tasks) == 0 && len(openspec.Changes(projectRoot)) == 0 {
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

	// Which specs already have a linked task (spec:<name> label)?
	linkedSpecs := make(map[string]bool)
	for _, t := range tasks {
		if s := specLabel(t.Labels); s != "" {
			linkedSpecs[s] = true
		}
	}

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	rows := make([][]string, 0)

	// Specs with no linked task yet — surfaced as items at the top.
	for _, ch := range openspec.Changes(projectRoot) {
		if linkedSpecs[ch.Name] {
			continue
		}
		status := "📋 spec"
		if ch.TotalTasks > 0 {
			status = fmt.Sprintf("📋 spec %d/%d", ch.CompletedTasks, ch.TotalTasks)
		}
		rows = append(rows, []string{ch.Name, "", "", status, "(no task — needs planning)"})
	}

	for _, t := range tasks {
		var status string
		if w, ok := workersByTask[t.ID]; ok {
			status = "🔨 " + w
		} else if t.Status == "in_progress" {
			status = "⚠ in_progress"
		} else {
			status = t.Status
		}
		updated := ""
		if ts, err := time.Parse(time.RFC3339Nano, t.UpdatedAt); err == nil {
			updated = ts.Local().Format("06-01-02 15:04")
		}
		title := t.Title
		if s := specLabel(t.Labels); s != "" {
			title = "📋 " + s + " · " + title
		}
		rows = append(rows, []string{t.ID, t.Priority, updated, status, title})

		for _, pr := range prByTask[t.ID] {
			rows = append(rows, []string{"", "", "", "", fmt.Sprintf("  └ %s [%s]", pr.ID, pr.Status)})
		}

		if gates := cliGateStatus(t.Labels); gates != "" {
			rows = append(rows, []string{"", "", "", "", "  " + gates})
		}
	}

	tbl := table.New().
		Headers("ID", "PRIO", "UPDATED", "STATUS", "TITLE").
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

// specLabel returns the spec name from a spec:<name> label, or "".
func specLabel(labels []string) string {
	for _, l := range labels {
		if strings.HasPrefix(l, "spec:") {
			return strings.TrimPrefix(l, "spec:")
		}
	}
	return ""
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
