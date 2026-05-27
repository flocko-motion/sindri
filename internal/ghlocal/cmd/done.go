package cmd

import (
	"fmt"
	"os"
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

		if out, err := exec.Command("git", "checkout", "--detach", base).CombinedOutput(); err != nil {
			return fmt.Errorf("checkout %s failed: %s", base, strings.TrimSpace(string(out)))
		}

		if err := os.WriteFile("/tmp/claude-status", []byte("idle"), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: update status file: %v\n", err)
		}
		fmt.Printf("Ready on %s. Run 'gh issue next' to pick up the next task.\n", base)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(doneCmd)
}
