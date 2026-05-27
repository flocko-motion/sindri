package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/flo-at/sindri/internal/ghlocal/store"
)

var prListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all local PRs",
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()
		prs, err := store.List()
		if err != nil {
			return err
		}
		if len(prs) == 0 {
			fmt.Println("No PRs found.")
			return nil
		}
		for _, pr := range prs {
			fmt.Printf("%-30s  %-8s  %s → %s\n", pr.ID, pr.Status, pr.Branch, pr.Base)
		}
		return nil
	},
}
