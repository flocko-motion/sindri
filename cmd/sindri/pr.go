package main

import (
	"os"

	ghcmd "github.com/flo-at/sindri/internal/ghlocal/cmd"
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
				os.Args = append([]string{"gh", "pr", "list"}, args...)
				ghcmd.Execute()
				return nil
			},
		},
		&cobra.Command{
			Use:   "view [id]",
			Short: "View a PR",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				os.Args = append([]string{"gh", "pr", "view"}, args...)
				ghcmd.Execute()
				return nil
			},
		},
		&cobra.Command{
			Use:   "approve [id]",
			Short: "Approve a PR",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				os.Args = append([]string{"gh", "pr", "review"}, append(args, "--approve")...)
				ghcmd.Execute()
				return nil
			},
		},
		&cobra.Command{
			Use:   "merge [id]",
			Short: "Merge an approved PR",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				os.Args = append([]string{"gh", "pr", "merge"}, args...)
				ghcmd.Execute()
				return nil
			},
		},
	)

	return prCmd
}
