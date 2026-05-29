// package: main (sindri) / lint
// type:    command
// job:     wires `sindri lint` — deadcode, loc, openspec, and all (run them all).
// limits:  Go analyses live in internal/lint, openspec validation in
//          adapter/spec; this only wires flags and exit codes.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/spec"
	"github.com/flo-at/sindri/internal/lint"
	"github.com/spf13/cobra"
)

// lintOpenspec validates the project's OpenSpec specs. It is a no-op (returns
// false) when openspec isn't used or installed. Returns true on validation
// failure.
func lintOpenspec(w io.Writer) bool {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(w, "openspec: cannot determine working dir: %v\n", err)
		return true
	}
	ok, out := spec.Validate(root)
	if out != "" {
		fmt.Fprint(w, out)
		if !strings.HasSuffix(out, "\n") {
			fmt.Fprintln(w)
		}
	}
	return !ok
}

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
			"non-zero when any unreachable function is found, so it can gate CI.\n\n" +
			"Add a //deadcode:keep comment directly above a function to keep it " +
			"(it will be excluded from the report).",
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgs := args
			if len(pkgs) == 0 {
				pkgs = []string{"./..."}
			}
			found, err := lint.Deadcode(pkgs, tags, includeTests, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			if found {
				os.Exit(1)
			}
			return nil
		},
	}
	deadcodeCmd.Flags().StringVar(&tags, "tags", "", "comma-separated list of extra build tags")
	deadcodeCmd.Flags().BoolVar(&includeTests, "test", false, "include test packages and executables")

	var maxLines int
	locCmd := &cobra.Command{
		Use:   "loc [dirs]",
		Short: "Report source files exceeding the line limit",
		Long: "Report Go source files longer than the per-file limit (default " +
			"700). Defaults to the current directory; exits non-zero when any " +
			"file is over the limit, so it can gate CI.",
		RunE: func(cmd *cobra.Command, args []string) error {
			found, err := lint.LOC(args, maxLines, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			if found {
				os.Exit(1)
			}
			return nil
		},
	}
	locCmd.Flags().IntVar(&maxLines, "max", lint.DefaultMaxLines, "maximum lines per file")

	openspecCmd := &cobra.Command{
		Use:   "openspec",
		Short: "Validate OpenSpec specs (no-op if openspec isn't used/installed)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if lintOpenspec(cmd.OutOrStdout()) {
				os.Exit(1)
			}
			return nil
		},
	}

	allCmd := &cobra.Command{
		Use:   "all",
		Short: "Run all linters and report (exits non-zero if any fail)",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			failed := false

			fmt.Fprintln(out, "== deadcode ==")
			dc, err := lint.Deadcode([]string{"./..."}, tags, includeTests, out)
			if err != nil {
				return err
			}
			failed = failed || dc

			fmt.Fprintln(out, "\n== loc ==")
			loc, err := lint.LOC([]string{"."}, maxLines, out)
			if err != nil {
				return err
			}
			failed = failed || loc

			fmt.Fprintln(out, "\n== openspec ==")
			failed = lintOpenspec(out) || failed

			fmt.Fprintln(out)
			if failed {
				fmt.Fprintln(out, "FAIL: lint violations found")
				os.Exit(1)
			}
			fmt.Fprintln(out, "OK: all linters passed")
			return nil
		},
	}
	allCmd.Flags().StringVar(&tags, "tags", "", "comma-separated list of extra build tags")
	allCmd.Flags().BoolVar(&includeTests, "test", false, "include test packages")
	allCmd.Flags().IntVar(&maxLines, "max", lint.DefaultMaxLines, "maximum lines per file")

	lintCmd.AddCommand(deadcodeCmd, locCmd, openspecCmd, allCmd)
	return lintCmd
}
