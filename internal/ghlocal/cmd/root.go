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

This is sindri's local git system — NOT the GitHub CLI.
There is no GitHub remote. All PRs are local.

Available commands:
  gh pr create    Create a local PR
  gh pr list      List local PRs
  gh pr view      View PR details
  gh pr review    Review a PR (--approve)
  gh pr merge     Merge an approved PR

Do NOT push, fetch, or use any GitHub-specific features.`, unknown)
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
