// package: agentcli / pr_review
// type:    command
// job:     `sindri-review pr approve|reject` — the reviewer agent's verdicts.
//          Approve satisfies the task's review gates; reject sends it back with
//          a reason. No human gate: these are the reviewer agent's job.
// limits:  the shared logic lives in internal/action; merge is human-only on the
//          host (-> cmd/sindri/pr.go).
package agentcli

import (
	"fmt"

	"github.com/flo-at/sindri/internal/action"
	"github.com/spf13/cobra"
)

var prApproveCmd = &cobra.Command{
	Use:   "approve [pr-id]",
	Short: "Approve a PR and satisfy its review gates",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()
		id, err := resolveID(args)
		if err != nil {
			return err
		}
		pr, err := action.Approve(tdProjectDir(), id)
		if err != nil {
			return err
		}
		fmt.Printf("Approved PR %s\n", pr.ID)
		return nil
	},
}

var rejectReason string

var prRejectCmd = &cobra.Command{
	Use:   "reject [pr-id]",
	Short: "Reject a PR back for rework (requires -m reason)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()
		id, err := resolveID(args)
		if err != nil {
			return err
		}
		pr, err := action.Reject(tdProjectDir(), id, rejectReason)
		if err != nil {
			return err
		}
		fmt.Printf("Rejected PR %s; task returned to open with feedback\n", pr.ID)
		return nil
	},
}

func init() {
	prRejectCmd.Flags().StringVarP(&rejectReason, "message", "m", "", "Reason for rejection (required)")
}
