package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/flo-at/sindri/internal/issue"
	"github.com/flo-at/sindri/internal/openspec"
	"github.com/flo-at/sindri/internal/render"
	"github.com/flo-at/sindri/internal/worker"
	"github.com/spf13/cobra"
)

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

	tasks, err := issue.LoadAll(projectRoot)
	if err != nil {
		return err
	}

	if !showAll {
		filtered := tasks[:0]
		for _, t := range tasks {
			switch {
			case showOpen && showClosed:
				filtered = append(filtered, t)
			case showClosed:
				if t.IsClosed() {
					filtered = append(filtered, t)
				}
			default: // default and --open both hide closed
				if !t.IsClosed() {
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
		if id := issue.TaskIDFromTitle(pr.Title); id != "" {
			prByTask[id] = append(prByTask[id], pr)
		}
	}

	// Which specs already have a linked task (spec:<name> label)?
	linkedSpecs := make(map[string]bool)
	for _, t := range tasks {
		if s := t.Spec(); s != "" {
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
			status = render.Worker(w)
		} else if t.Status == "in_progress" {
			status = render.Orphaned()
		} else {
			status = render.TaskStatus(t.Status)
		}
		updated := ""
		if !t.UpdatedAt.IsZero() {
			updated = t.UpdatedAt.Local().Format("06-01-02 15:04")
		}
		title := t.Title
		if s := t.Spec(); s != "" {
			title = "📋 " + s + " · " + title
		}
		rows = append(rows, []string{t.ID, t.Priority, updated, status, title})

		for _, pr := range prByTask[t.ID] {
			rows = append(rows, []string{"", "", "", "", "  └ " + pr.ID + " [" + render.PRStatus(pr.Status, t.IsClosed()) + "]"})
		}

		if gates := render.Gates(t.Gates()); gates != "" {
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
		if issue.TaskIDFromTitle(pr.Title) == taskID {
			fmt.Printf("PR: %s [%s] %s → %s\n", pr.ID, pr.Status, pr.Branch, pr.Base)
		}
	}

	return nil
}
