package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/flo-at/sindri/internal/ghlocal/store"
)

var prMergeCmd = &cobra.Command{
	Use:   "merge [pr-id]",
	Short: "Merge an approved local PR",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()
		id, err := resolveID(args)
		if err != nil {
			return err
		}
		pr, err := store.Merge(id)
		if err != nil {
			return err
		}
		fmt.Printf("Merged PR %s into %s\n", pr.ID, pr.Base)
		return nil
	},
}
