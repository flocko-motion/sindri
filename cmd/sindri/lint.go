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
	"runtime/debug"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/spec"
	"github.com/flo-at/sindri/internal/lint"
	"github.com/spf13/cobra"
)

// runLint runs one linter's body and always finishes with an "=== EXIT N ==="
// marker on out, so a caller (often an agent) reading the output learns the
// result without appending its own `echo "$?"`. It also recovers from a panic,
// turning it into a loud EXIT 1 with the stack rather than an opaque crash with
// no marker. fn reports whether violations were found and any hard error; either
// means failure. On failure it exits non-zero; on success it returns so the
// normal command teardown still runs.
func runLint(out io.Writer, fn func() (bool, error)) error {
	if lintOutcome(out, fn) != 0 {
		os.Exit(1)
	}
	return nil
}

// lintOutcome runs fn under a panic recover, reports the failure reason (a hard
// error or a panic with its stack) to out, always prints the "=== EXIT N ==="
// marker, and returns the exit code (1 if violations were found, an error
// occurred, or fn panicked; else 0). Split from runLint so it's testable without
// os.Exit.
func lintOutcome(out io.Writer, fn func() (bool, error)) int {
	code := func() (code int) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(out, "panic: %v\n\n%s\n", r, debug.Stack())
				code = 1
			}
		}()
		failed, err := fn()
		switch {
		case err != nil:
			fmt.Fprintf(out, "error: %v\n", err)
			return 1
		case failed:
			return 1
		}
		return 0
	}()
	fmt.Fprintf(out, "=== EXIT %d ===\n", code)
	return code
}

// commentsConvention explains what `lint comments` expects, shown both in its
// --help and after its violations so the fix is obvious without leaving the
// terminal.
const commentsConvention = `Expected (architecture spec "File headers", plus documented exports):

  - Every non-test .go file opens with a four-field header comment block,
    directly above the package clause — the same block "code map" reads.
  - Every exported function and type carries at least one line of doc comment.

Example:

    // package: widget / build
    // type:    logic
    // job:     assembles widgets from their parts
    // limits:  doesn't render them (-> render)
    package widget

    // Widget is an assembled tree of parts.
    type Widget struct{}

    // Build assembles a Widget from the given parts.
    func Build(parts ...Part) *Widget { return nil }`

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
	deadcodeCmd := &cobra.Command{
		Use:   "deadcode [packages]",
		Short: "Report unreachable functions (exits non-zero if any are found)",
		Long: "Report source-level functions unreachable from any main package, " +
			"computed via Rapid Type Analysis. Defaults to ./... and exits " +
			"non-zero when any unreachable function is found, so it can gate CI. " +
			"Test packages are always analysed (tests are live code).\n\n" +
			"Add a //deadcode:keep comment directly above a function to keep it " +
			"(it will be excluded from the report).",
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgs := args
			if len(pkgs) == 0 {
				pkgs = []string{"./..."}
			}
			return runLint(cmd.OutOrStdout(), func() (bool, error) {
				return lint.Deadcode(pkgs, tags, cmd.OutOrStdout())
			})
		},
	}
	deadcodeCmd.Flags().StringVar(&tags, "tags", "", "comma-separated list of extra build tags")

	var maxLines int
	locCmd := &cobra.Command{
		Use:   "loc [dirs]",
		Short: "Report source files exceeding the line limit",
		Long: "Report Go source files longer than the per-file limit (default " +
			"700). Defaults to the current directory; exits non-zero when any " +
			"file is over the limit, so it can gate CI.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLint(cmd.OutOrStdout(), func() (bool, error) {
				return lint.LOC(args, maxLines, cmd.OutOrStdout())
			})
		},
	}
	locCmd.Flags().IntVar(&maxLines, "max", lint.DefaultMaxLines, "maximum lines per file")

	commentsCmd := &cobra.Command{
		Use:   "comments [dirs]",
		Short: "Report missing canonical headers and undocumented exported funcs/types",
		Long: "Report non-test Go files that lack the canonical four-field header " +
			"(package/type/job/limits, the same block `code map` reads), and " +
			"exported functions and types with no doc comment. Defaults to the " +
			"current directory; exits non-zero on any violation, so it can gate CI.\n\n" +
			commentsConvention,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			return runLint(out, func() (bool, error) {
				found, err := lint.Comments(args, out)
				if err != nil {
					return false, err
				}
				if found {
					// Follow the violations with what's expected, so the fix is obvious
					// without having to go read --help or the spec.
					fmt.Fprintf(out, "\n%s\n", commentsConvention)
				}
				return found, nil
			})
		},
	}

	openspecCmd := &cobra.Command{
		Use:   "openspec",
		Short: "Validate OpenSpec specs (no-op if openspec isn't used/installed)",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			return runLint(out, func() (bool, error) {
				return lintOpenspec(out), nil
			})
		},
	}

	allCmd := &cobra.Command{
		Use:   "all",
		Short: "Run all linters and report (exits non-zero if any fail)",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			return runLint(out, func() (bool, error) {
				failed := false

				fmt.Fprintln(out, "== deadcode ==")
				dc, err := lint.Deadcode([]string{"./..."}, tags, out)
				if err != nil {
					return false, err
				}
				failed = failed || dc

				fmt.Fprintln(out, "\n== loc ==")
				loc, err := lint.LOC([]string{"."}, maxLines, out)
				if err != nil {
					return false, err
				}
				failed = failed || loc

				fmt.Fprintln(out, "\n== comments ==")
				cm, err := lint.Comments([]string{"."}, out)
				if err != nil {
					return false, err
				}
				if cm {
					fmt.Fprintf(out, "\n%s\n", commentsConvention)
				}
				failed = failed || cm

				fmt.Fprintln(out, "\n== openspec ==")
				failed = lintOpenspec(out) || failed

				fmt.Fprintln(out)
				if failed {
					fmt.Fprintln(out, "FAIL: lint violations found")
				} else {
					fmt.Fprintln(out, "OK: all linters passed")
				}
				return failed, nil
			})
		},
	}
	allCmd.Flags().StringVar(&tags, "tags", "", "comma-separated list of extra build tags")
	allCmd.Flags().IntVar(&maxLines, "max", lint.DefaultMaxLines, "maximum lines per file")

	lintCmd.AddCommand(deadcodeCmd, locCmd, commentsCmd, openspecCmd, allCmd)
	return lintCmd
}
