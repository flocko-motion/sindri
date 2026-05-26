package cmd

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/sysiphos/gh-local/internal/store"
)

var prMergeCmd = &cobra.Command{
	Use:   "merge [pr-id]",
	Short: "Merge an approved local PR",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := resolveID(args)
		if err != nil {
			return err
		}

		pr, err := store.Read(id)
		if err != nil {
			return err
		}

		if pr.Status != "approved" {
			return fmt.Errorf("PR %s is not approved (status: %s) — merge blocked", id, pr.Status)
		}

		// Switch to base and merge
		if out, err := exec.Command("git", "checkout", pr.Base).CombinedOutput(); err != nil {
			return fmt.Errorf("checkout %s failed: %s", pr.Base, out)
		}
		if out, err := exec.Command("git", "merge", "--no-ff", pr.Branch, "-m",
			fmt.Sprintf("Merge PR %s: %s", id, pr.Title)).CombinedOutput(); err != nil {
			return fmt.Errorf("merge failed: %s", out)
		}

		// Cleanup: delete the branch
		exec.Command("git", "branch", "-d", pr.Branch).Run() //nolint:errcheck

		pr.Status = "merged"
		if err := store.Write(pr); err != nil {
			return err
		}

		fmt.Printf("Merged PR %s into %s\n", id, pr.Base)
		return nil
	},
}
