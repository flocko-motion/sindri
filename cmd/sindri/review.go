// package: main (sindri) / review
// type:    command
// job:     wires `sindri review`, launching the reviewer worker container.
// limits:  container lifecycle lives in internal/worker; nothing here but wiring.
package main

import (
	"fmt"

	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/worker"
	"github.com/spf13/cobra"
)

func newReviewCmd() *cobra.Command {
	var shellMode bool
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Start a reviewer (alias for 'worker review')",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReview(shellMode)
		},
	}
	cmd.Flags().BoolVar(&shellMode, "shell", false, "Open a shell instead of launching claude (for debugging)")
	return cmd
}

func runReview(shell bool) error {
	projectRoot, err := worker.GitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}
	if err := container.Ensure(projectRoot); err != nil {
		return err
	}
	return worker.StartReviewer(projectRoot, shell)
}
