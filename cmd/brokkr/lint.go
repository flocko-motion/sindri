// package: main (brokkr) / lint
// type:    command
// job:     wires `brokkr lint` — no argument runs every linter (deadcode, loc,
//          comments, openspec) with a summary; `brokkr lint <name>` runs one.
//          Exits non-zero on any violation, so it can gate CI; add --tail to also
//          print the exit status inline.
// limits:  Go analyses live in internal/lint, openspec validation in
//          adapter/spec; this only wires flags, dispatch, and exit codes.
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

func newLintCmd() *cobra.Command {
	var tags string
	var maxLines int
	var ignore []string
	c := &cobra.Command{
		Use:   "lint [linter]",
		Short: "Run the quality gate: lint (all) or lint <deadcode|loc|comments|openspec>",
		Long: "Run the project's static-analysis linters. With no argument, runs them " +
			"all (deadcode, loc, comments, openspec) with a summary; with a linter " +
			"name, runs just that one. Exits non-zero on any violation, so it can gate " +
			"CI (add --tail to also print the exit status inline).\n\n" +
			"Use --ignore to exclude files you can't fix (e.g. generated code): a " +
			"pattern with no '/' matches a basename at any depth (--ignore='*.gen.go'), " +
			"one containing '/' matches the relative path with '*'/'**' wildcards " +
			"(--ignore='internal/gen/**'), and a 're:' prefix is a Go regexp. Repeat " +
			"the flag for several patterns. It applies to the Go linters, not openspec.\n\n" +
			commentsConvention,
		Args: cobra.MaximumNArgs(1),
		// lint reports failures itself and signals them with an exitCodeError (empty
		// message); silence cobra's own error echo for it.
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			which := ""
			if len(args) == 1 {
				which = args[0]
			}
			ig, err := lint.NewIgnore(ignore)
			if err != nil {
				return err
			}
			return runLint(out, func() (bool, error) {
				return runLinters(out, which, tags, maxLines, ig)
			})
		},
	}
	c.Flags().StringVar(&tags, "tags", "", "comma-separated list of extra build tags (deadcode)")
	c.Flags().IntVar(&maxLines, "max", lint.DefaultMaxLines, "maximum lines per file (loc)")
	c.Flags().StringArrayVar(&ignore, "ignore", nil, "skip files matching this glob (no '/' = basename anywhere) or 're:'-prefixed regexp; repeatable")
	return c
}

// runLinters runs the named linter, or all of them when which is empty. Returns
// whether any violation was found.
func runLinters(out io.Writer, which, tags string, maxLines int, ig *lint.Ignore) (bool, error) {
	switch which {
	case "deadcode":
		return lint.Deadcode([]string{"./..."}, tags, ig, out)
	case "loc":
		return lint.LOC([]string{"."}, maxLines, ig, out)
	case "comments":
		return runComments(out, ig)
	case "openspec":
		return lintOpenspec(out), nil
	case "":
		return runAll(out, tags, maxLines, ig)
	default:
		return false, fmt.Errorf("unknown linter %q (want deadcode|loc|comments|openspec)", which)
	}
}

// runComments runs the documentation linter and, on a violation, follows it with
// the convention so the fix is obvious without leaving the terminal.
func runComments(out io.Writer, ig *lint.Ignore) (bool, error) {
	found, err := lint.Comments([]string{"."}, ig, out)
	if err != nil {
		return false, err
	}
	if found {
		fmt.Fprintf(out, "\n%s\n", commentsConvention)
	}
	return found, nil
}

// runAll runs every linter in turn and prints a section per linter plus a final
// PASS/FAIL summary.
func runAll(out io.Writer, tags string, maxLines int, ig *lint.Ignore) (bool, error) {
	failed := false

	fmt.Fprintln(out, "== deadcode ==")
	dc, err := lint.Deadcode([]string{"./..."}, tags, ig, out)
	if err != nil {
		return false, err
	}
	failed = failed || dc

	fmt.Fprintln(out, "\n== loc ==")
	loc, err := lint.LOC([]string{"."}, maxLines, ig, out)
	if err != nil {
		return false, err
	}
	failed = failed || loc

	fmt.Fprintln(out, "\n== comments ==")
	cm, err := runComments(out, ig)
	if err != nil {
		return false, err
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
}

// runLint runs one linter's body, recovering a panic into a loud failure with its
// stack rather than an opaque crash. On failure it returns an exitCodeError so main
// exits non-zero (never os.Exit here — that would bypass the --tail flush); on
// success it returns nil. Use --tail to get the exit status printed inline.
func runLint(out io.Writer, fn func() (bool, error)) error {
	if code := lintOutcome(out, fn); code != 0 {
		return exitCodeError{code}
	}
	return nil
}

// lintOutcome runs fn under a panic recover, reports the failure reason (a hard
// error or a panic with its stack) to out, and returns the exit code (1 if
// violations were found, an error occurred, or fn panicked; else 0). Split from
// runLint so it's testable in isolation.
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
	return code
}

// commentsConvention explains what the comments linter expects, shown both in
// --help and after its violations so the fix is obvious without leaving the
// terminal.
const commentsConvention = `Expected (architecture spec "File headers", plus documented exports):

  - Every non-test .go file opens with a four-field header comment block,
    directly above the package clause — the same block "brokkr map" reads.
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
