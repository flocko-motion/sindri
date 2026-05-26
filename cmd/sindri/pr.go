package main

import (
	"fmt"
	"os"
	"text/tabwriter"

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
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				for _, pr := range prs {
					fmt.Fprintf(w, "%s\t%s\t%s → %s\n", pr.ID, pr.Status, pr.Branch, pr.Base)
				}
				w.Flush()
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
				pr, err := store.Merge(args[0])
				if err != nil {
					return err
				}
				fmt.Printf("Merged PR %s into %s\n", pr.ID, pr.Base)
				return nil
			},
		},
	)

	return prCmd
}
