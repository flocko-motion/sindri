package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var doneCmd = &cobra.Command{
	Use:   "done",
	Short: "Return to base branch, ready for next task",
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()

		base := baseBranch()
		branch, _ := currentBranch()

		if branch == base {
			fmt.Printf("Already on %s. Run 'gh next' to pick up a task.\n", base)
			return nil
		}

		if out, err := exec.Command("git", "checkout", base).CombinedOutput(); err != nil {
			return fmt.Errorf("checkout %s failed: %s", base, strings.TrimSpace(string(out)))
		}

		fmt.Printf("Checked out %s. Run 'gh next' to pick up the next task.\n", base)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(doneCmd)
}
