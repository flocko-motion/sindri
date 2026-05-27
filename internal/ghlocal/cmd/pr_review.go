package cmd

import (
	"fmt"

	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/spf13/cobra"
)

var (
	reviewApprove bool
	reviewReject  bool
)

var prReviewCmd = &cobra.Command{
	Use:   "review [pr-id]",
	Short: "Review a local PR (--approve or --reject)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()
		id, err := resolveID(args)
		if err != nil {
			return err
		}

		if !reviewApprove && !reviewReject {
			return fmt.Errorf("specify --approve or --reject")
		}

		if reviewApprove {
			pr, err := store.Approve(id)
			if err != nil {
				return err
			}
			fmt.Printf("Approved PR: %s\n", pr.ID)
		}

		if reviewReject {
			pr, err := store.Read(id)
			if err != nil {
				return err
			}
			pr.Status = "rejected"
			if err := store.Write(pr); err != nil {
				return err
			}
			fmt.Printf("Rejected PR: %s\n", pr.ID)
		}

		return nil
	},
}

func init() {
	prReviewCmd.Flags().BoolVar(&reviewApprove, "approve", false, "Approve the PR")
	prReviewCmd.Flags().BoolVar(&reviewReject, "reject", false, "Reject the PR")
}
