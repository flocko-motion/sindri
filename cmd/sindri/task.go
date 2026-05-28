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
	"github.com/flo-at/sindri/internal/ghlocal/store"
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

	issues, err := board.List(projectRoot)
	if err != nil {
		return err
	}

	if !showAll {
		filtered := issues[:0]
		for _, iss := range issues {
			switch {
			case showOpen && showClosed:
				filtered = append(filtered, iss)
			case showClosed:
				if iss.IsClosed() {
					filtered = append(filtered, iss)
				}
			default: // default and --open both hide closed (spec-only is never closed)
				if !iss.IsClosed() {
					filtered = append(filtered, iss)
				}
			}
		}
		issues = filtered
	}

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
		rows = append(rows, []string{iss.ID(), iss.Priority(), updated, render.IssueStatus(iss), iss.Title()})

		for _, pr := range iss.PRs {
			rows = append(rows, []string{"", "", "", "", "  └ " + pr.ID + " [" + render.PRStatus(pr.Status, iss.IsClosed()) + "]"})
		}

		if gates := render.Gates(iss.Gates()); gates != "" {
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
	out, err := td.Show(projectRoot, taskID)
	if err != nil {
		return fmt.Errorf("task %s not found", taskID)
	}
	fmt.Println(out)

	prs, _ := store.ListFor(projectRoot)
	fmt.Println()
	for _, pr := range prs {
		if issue.TaskIDFromTitle(pr.Title) == taskID {
			fmt.Printf("PR: %s [%s] %s → %s\n", pr.ID, pr.Status, pr.Branch, pr.Base)
		}
	}

	return nil
}
