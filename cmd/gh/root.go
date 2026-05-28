// package: gh / root
// type:    entrypoint
// job:     the sindri-worker command tree and the "not GitHub" unknown-command
//          help. Agents drive the workflow through these subcommands.
// limits:  subcommand behavior lives in the sibling files; storage in store.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "sindri-worker",
	Short:        "sindri-worker — local workflow engine for sandboxed agents (NOT GitHub)",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		unknown := strings.Join(args, " ")
		return fmt.Errorf(`unknown command: sindri-worker %s

This is sindri-worker — a local workflow engine, NOT the GitHub CLI.
There is no GitHub remote. All operations are local.

Workflow commands:
  sindri-worker issue next       Pick up the next task (claim, branch, show details)
  sindri-worker issue list       List tasks
  sindri-worker issue view <id>  Show task details + comments + PR status
  sindri-worker issue comment    Add a comment to a task
  sindri-worker submit           Submit work (rebase, lint, create PR, handoff)
  sindri-worker done             Return to base branch for next task

PR commands:
  sindri-worker pr list          List local PRs
  sindri-worker pr view          View a PR
  sindri-worker pr create        Create a PR (prefer 'sindri-worker submit' instead)
  sindri-worker pr review        Approve a PR
  sindri-worker pr merge         Merge an approved PR`, unknown)
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
