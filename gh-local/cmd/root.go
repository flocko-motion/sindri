package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "gh",
	Short:        "gh-local — a gh wrapper for sandboxed agent work",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		unknown := strings.Join(args, " ")
		return fmt.Errorf(`unknown command: gh %s

This is gh-local, a lightweight gh wrapper for sandboxed agent work.
It implements a limited subset of the gh CLI:

  gh pr create    Create a PR from the current branch
  gh pr list      List all PRs
  gh pr view      View PR details
  gh pr review    Review a PR (--approve)
  gh pr merge     Merge an approved PR

Commands outside this set are not supported.`, unknown)
	},
	Args: cobra.ArbitraryArgs,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(prCmd)
}
