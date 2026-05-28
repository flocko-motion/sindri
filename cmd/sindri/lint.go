package main

import (
	"os"

	"github.com/flo-at/sindri/internal/lint"
	"github.com/spf13/cobra"
)

func newLintCmd() *cobra.Command {
	lintCmd := &cobra.Command{
		Use:   "lint",
		Short: "Static-analysis linters",
	}

	var tags string
	var includeTests bool
	deadcodeCmd := &cobra.Command{
		Use:   "deadcode [packages]",
		Short: "Report unreachable functions (exits non-zero if any are found)",
		Long: "Report source-level functions unreachable from any main package, " +
			"computed via Rapid Type Analysis. Defaults to ./... and exits " +
			"non-zero when any unreachable function is found, so it can gate CI.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				args = []string{"./..."}
			}
			found, err := lint.Deadcode(args, tags, includeTests, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			if found {
				// Reported lines are the output; exit non-zero without an
				// extra error message so this works cleanly as a gate.
				os.Exit(1)
			}
			return nil
		},
	}
	deadcodeCmd.Flags().StringVar(&tags, "tags", "", "comma-separated list of extra build tags")
	deadcodeCmd.Flags().BoolVar(&includeTests, "test", false, "include test packages and executables")
	lintCmd.AddCommand(deadcodeCmd)
	return lintCmd
}
