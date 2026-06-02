// package: main (sindri) / task
// type:    command
// job:     the `sindri task` subcommands (list/new/view/comment); renders
//          board.List issues and creates/reads tasks via the td adapter.
// limits:  no domain logic (-> issue/board), no styling rules (-> render),
//          no direct td calls (-> adapter/td).
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/board"
	"github.com/flo-at/sindri/internal/issue"
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
			root, err := worker.GitRoot()
			if err != nil {
				return err
			}
			if err := td.Comment(root, args[0], msg); err != nil {
				return err
			}
			fmt.Printf("Comment added to %s\n", args[0])
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
			root, err := worker.GitRoot()
			if err != nil {
				return err
			}
			var labels []string
			if newReview {
				labels = append(labels, "require-review-code")
			}
			if newSpec != "" {
				labels = append(labels, "spec:"+newSpec)
			}
			out, err := td.Create(root, args[0], td.CreateOpts{
				Type: newType, Priority: newPrio, Body: newBody, Labels: labels,
			})
			if err != nil {
				return err
			}
			fmt.Println(out)
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

	// One-shot CLI: warm the parent_id cache so hierarchy renders in this run.
	// The TUI gets this in the background; the CLI accepts the latency.
	board.WarmParentCache(projectRoot)

	issues, err := board.List(projectRoot)
	if err != nil {
		return err
	}

	filter := issue.FilterOpen
	switch {
	case showAll || (showOpen && showClosed):
		filter = issue.FilterAll
	case showClosed:
		filter = issue.FilterClosed
	}
	issues = issue.Apply(issues, filter)

	if len(issues) == 0 {
		fmt.Println("No tasks found.")
		return nil
	}

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	rows := make([][]string, 0)

	for _, iss := range issues {
		updated := ""
		if !iss.UpdatedAt().IsZero() {
			updated = iss.UpdatedAt().Local().Format("06-01-02 15:04")
		}
		rows = append(rows, []string{render.TypeColumn(iss), iss.ID(), iss.Priority(), updated, render.IssueStatus(iss), iss.Title()})

		for _, pr := range iss.PRs {
			rows = append(rows, []string{"", "", "", "", "", "  └ " + pr.ID + " [" + render.PRStatus(pr.Status, iss.IsClosed()) + "]"})
		}

		if gates := render.Gates(iss.Gates()); gates != "" {
			rows = append(rows, []string{"", "", "", "", "", "  " + gates})
		}
	}

	tbl := table.New().
		Headers("TYPE", "ID", "PRIO", "UPDATED", "STATUS", "TITLE").
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

	issues, err := board.List(projectRoot)
	if err != nil {
		return err
	}
	var iss *issue.Issue
	for i := range issues {
		if issues[i].ID() == taskID {
			iss = &issues[i]
			break
		}
	}
	if iss == nil {
		return fmt.Errorf("issue %s not found", taskID)
	}

	if iss.SpecOnly() {
		fmt.Printf("ID:     %s\n", iss.ID())
		fmt.Printf("Spec:   %s\n", iss.Spec.Name)
		if done, total, _ := iss.SpecProgress(); total > 0 {
			fmt.Printf("Tasks:  %d/%d\n", done, total)
		} else {
			fmt.Printf("Tasks:  none yet\n")
		}
		return nil
	}

	t := iss.Task
	fmt.Printf("ID:       %s\n", iss.ID())
	fmt.Printf("Status:   %s\n", render.TaskStatus(t.Status))
	fmt.Printf("Type:     %s\n", t.Type)
	fmt.Printf("Priority: %s\n", t.Priority)
	if iss.Spec != nil {
		fmt.Printf("Spec:     %s\n", iss.Spec.Name)
	}
	if iss.Worker != "" {
		fmt.Printf("Worker:   %s\n", iss.Worker)
	}
	if gates := render.Gates(iss.Gates()); gates != "" {
		fmt.Printf("Gates:    %s\n", gates)
	}
	if !t.CreatedAt.IsZero() {
		fmt.Printf("Created:  %s\n", t.CreatedAt.Local().Format("2006-01-02 15:04"))
	}
	if !t.UpdatedAt.IsZero() {
		fmt.Printf("Updated:  %s\n", t.UpdatedAt.Local().Format("2006-01-02 15:04"))
	}

	if len(iss.PRs) > 0 {
		fmt.Println("\nPull Requests:")
		for _, pr := range iss.PRs {
			fmt.Printf("  %s [%s] %s → %s\n", pr.ID, render.PRStatus(pr.Status, iss.IsClosed()), pr.Branch, pr.Base)
		}
	}

	if desc, err := td.Show(projectRoot, taskID); err == nil && desc != "" {
		fmt.Printf("\n%s\n", desc)
	}
	if c, err := td.Comments(projectRoot, taskID); err == nil && c != "" && c != "No comments" {
		fmt.Printf("\n--- Comments ---\n%s\n", c)
	}
	return nil
}
