// package: gh / done
// type:    command
// job:     the agent's `sindri-worker done` — return to base branch, ready for next.
// limits:  no task/PR mutation; just git branch state and the status file.
package main

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
			fmt.Printf("Already on %s. Run 'sindri-worker issue next' to pick up a task.\n", base)
			return nil
		}

		if out, err := exec.Command("git", "checkout", "--detach", base).CombinedOutput(); err != nil {
			return fmt.Errorf("checkout %s failed: %s", base, strings.TrimSpace(string(out)))
		}

		if err := os.WriteFile("/tmp/claude-status", []byte("idle"), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: update status file: %v\n", err)
		}
		if err := os.Remove(".sindri-task"); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: remove .sindri-task: %v\n", err)
		}
		fmt.Printf("Ready on %s. Run 'sindri-worker issue next' to pick up the next task.\n", base)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(doneCmd)
}
