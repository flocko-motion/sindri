package cmd

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
