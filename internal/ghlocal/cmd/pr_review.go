package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/flo-at/sindri/internal/ghlocal/store"
)

var reviewApprove bool

var prReviewCmd = &cobra.Command{
	Use:   "review [pr-id]",
	Short: "Review a local PR",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()
		id, err := resolveID(args)
		if err != nil {
			return err
		}

		if !reviewApprove {
			return fmt.Errorf("specify --approve to approve the PR")
		}

		pr, err := store.Approve(id)
		if err != nil {
			return err
		}
		fmt.Printf("Approved PR: %s\n", pr.ID)
		return nil
	},
}

func init() {
	prReviewCmd.Flags().BoolVar(&reviewApprove, "approve", false, "Approve the PR")
}
