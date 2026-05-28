// package: agentcli / pr_review
// type:    command
// job:     `sindri-review pr approve|reject` — the reviewer agent's verdicts.
//          Approve satisfies the task's review gates; reject sends it back with
//          a reason. No human gate: these are the reviewer agent's job.
// limits:  PR records in store; task labels/state via the td adapter. Merge is
//          human-only on the host (-> cmd/sindri/pr.go).
package agentcli

import (
	"fmt"

	tdadapter "github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/ghlocal/store"
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
		pr, err := store.Approve(id)
		if err != nil {
			return err
		}

		// Satisfy the task's review gates by adding the matching approved-* labels.
		root := tdProjectDir()
		taskID := pr.Branch
		task, err := tdadapter.Get(root, taskID)
		if err != nil {
			return fmt.Errorf("approved PR %s but could not load task %s to mark gates: %w", pr.ID, taskID, err)
		}
		labels := append([]string{}, task.Labels...)
		marked := false
		for _, g := range task.Gates() {
			if !g.Approved {
				labels = append(labels, "approved-"+g.Name)
				marked = true
			}
		}
		if marked {
			if err := tdadapter.SetLabels(root, taskID, labels); err != nil {
				return fmt.Errorf("approved PR %s but failed to mark gates on %s: %w", pr.ID, taskID, err)
			}
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
		if rejectReason == "" {
			return fmt.Errorf("a reason is required: -m \"why\"")
		}
		id, err := resolveID(args)
		if err != nil {
			return err
		}
		pr, err := store.Read(id)
		if err != nil {
			return err
		}
		root := tdProjectDir()
		taskID := pr.Branch
		if err := tdadapter.Comment(root, taskID, "Review rejected: "+rejectReason); err != nil {
			return fmt.Errorf("failed to comment the reason on %s: %w", taskID, err)
		}
		pr.Status = "rejected"
		if err := store.Write(pr); err != nil {
			return err
		}
		if err := tdadapter.Reject(root, taskID); err != nil {
			return fmt.Errorf("marked PR %s rejected but failed to reopen task %s: %w", pr.ID, taskID, err)
		}
		fmt.Printf("Rejected PR %s; task %s returned to open with feedback\n", pr.ID, taskID)
		return nil
	},
}

func init() {
	prRejectCmd.Flags().StringVarP(&rejectReason, "message", "m", "", "Reason for rejection (required)")
}
