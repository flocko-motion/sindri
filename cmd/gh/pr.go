// package: gh / pr
// type:    command
// job:     the `gh pr` subcommand group (create/list/view/review/merge).
// limits:  group wiring; each operation lives in its own pr_*.go file.
package main

import (
	"github.com/spf13/cobra"
)

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Manage local pull requests",
}

func init() {
	prCmd.AddCommand(prCreateCmd)
	prCmd.AddCommand(prReviewCmd)
	prCmd.AddCommand(prMergeCmd)
	prCmd.AddCommand(prListCmd)
	prCmd.AddCommand(prViewCmd)
}
