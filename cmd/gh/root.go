// package: gh / root
// type:    entrypoint
// job:     the gh (sindri-local) command tree and the "not GitHub" unknown-
//          command help. Agents drive the workflow through these subcommands.
// limits:  subcommand behavior lives in the sibling files; storage in store.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "gh",
	Short:        "sindri-local — workflow engine for sandboxed agents (not GitHub)",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		unknown := strings.Join(args, " ")
		return fmt.Errorf(`unknown command: gh %s

This is sindri-local — NOT the GitHub CLI.
There is no GitHub remote. All operations are local.

Workflow commands:
  gh issue next       Pick up the next task (claim, branch, show details)
  gh issue list       List tasks
  gh issue view <id>  Show task details + comments + PR status
  gh issue comment    Add a comment to a task
  gh submit           Submit work (rebase, create PR, handoff, wait for review)
  gh done             Return to base branch for next task

PR commands:
  gh pr list          List local PRs
  gh pr view          View a PR
  gh pr create        Create a PR (prefer 'gh submit' instead)
  gh pr review        Approve a PR
  gh pr merge         Merge an approved PR`, unknown)
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
