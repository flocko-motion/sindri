// package: gh / pr_list
// type:    command
// job:     `sindri-worker pr list` — lists local PRs.
// limits:  PR records live in internal/ghlocal/store.
package main

import (
	"fmt"

	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/spf13/cobra"
)

var prListAll bool

var prListCmd = &cobra.Command{
	Use:   "list",
	Short: "List local PRs (open + approved by default, --all for everything)",
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()
		prs, err := store.List()
		if err != nil {
			return err
		}
		count := 0
		for _, pr := range prs {
			if !prListAll && pr.Status != "open" && pr.Status != "approved" {
				continue
			}
			fmt.Printf("%-30s  %-8s  %s → %s\n", pr.ID, pr.Status, pr.Branch, pr.Base)
			count++
		}
		if count == 0 {
			fmt.Println("No PRs found.")
		}
		return nil
	},
}

func init() {
	prListCmd.Flags().BoolVar(&prListAll, "all", false, "Show all PRs, not just open")
}
