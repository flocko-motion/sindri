package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/sysiphos/gh-local/internal/store"
)

var reviewApprove bool

var prReviewCmd = &cobra.Command{
	Use:   "review [pr-id]",
	Short: "Review a local PR",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := resolveID(args)
		if err != nil {
			return err
		}

		if !reviewApprove {
			return fmt.Errorf("specify --approve to approve the PR")
		}

		pr, err := store.Read(id)
		if err != nil {
			return err
		}
		if pr.Status == "merged" {
			return fmt.Errorf("PR %s is already merged", id)
		}

		pr.Status = "approved"
		if err := store.Write(pr); err != nil {
			return err
		}

		fmt.Printf("Approved PR: %s\n", id)
		return nil
	},
}

func init() {
	prReviewCmd.Flags().BoolVar(&reviewApprove, "approve", false, "Approve the PR")
}
