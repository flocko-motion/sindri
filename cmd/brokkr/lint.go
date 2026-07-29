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

	"github.com/flo-at/sindri/internal/adapter/tasks/spec"
	"github.com/flo-at/sindri/internal/brokkr/lint"
	"github.com/flo-at/sindri/internal/config"
	"github.com/spf13/cobra"
)

func newLintCmd() *cobra.Command {
	var tags string
	var maxLines int
	var maxAvg float64
	var ignore []string
	c := &cobra.Command{
		Use:   "lint [linter]",
		Short: "Run the quality gate: lint (all) or lint <deadcode|loc|comments|comment-length|js|openspec>",
		Long: "Run the project's static-analysis linters. With no argument, runs them " +
			"all (deadcode, loc, comments, comment-length, js, openspec) with a summary; with a linter " +
			"name, runs just that one. Exits non-zero on any violation, so it can gate " +
			"CI (add --tail to also print the exit status inline).\n\n" +
			"Use --ignore to exclude files you can't fix (e.g. generated code): a " +
			"pattern with no '/' matches a basename at any depth (--ignore='*.gen.go'), " +
			"one containing '/' matches the relative path with '*'/'**' wildcards " +
			"(--ignore='internal/gen/**'), and a 're:' prefix is a Go regexp. Repeat " +
			"the flag for several patterns. It applies to the Go linters, not openspec.\n\n" +
			"The openspec linter is not brokkr's own: it runs " + spec.ValidatorName + ", so a " +
			"green result here is the same green result the submit gate gives — there is no " +
			"second, stricter validator to satisfy afterwards. Note that `openspec validate " +
			"--strict` WITHOUT --all validates nothing and exits 0, which reads as a pass.\n\n" +
			"For permanent exceptions, commit a " + lint.IgnoreFileName + " file at the " +
			"repo root: one pattern per line (same syntax; '#' comments and blank lines " +
			"ignored). It's read automatically by every run — the right home for a " +
			"generated file's exception, since the file itself can't carry a marker.\n\n" +
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
			// Merge the repo's checked-in .brokkrignore (exceptions for generated
			// files that can't carry an in-file marker) with any --ignore flags.
			filePats, err := lint.LoadIgnoreFile(".")
			if err != nil {
				return err
			}
			ig, err := lint.NewIgnore(append(filePats, ignore...))
			if err != nil {
				return err
			}
			// The repo's own bar, where it set one: a house style belongs to the house. An
			// explicit flag still wins, so a one-off run can override it.
			lines, avg := repoLintBar(cmd, maxLines, maxAvg)
			return runLint(out, func() (bool, error) {
				return runLinters(out, which, tags, lines, avg, ig)
			})
		},
	}
	c.Flags().StringVar(&tags, "tags", "", "comma-separated list of extra build tags (deadcode)")
	c.Flags().IntVar(&maxLines, "max", lint.DefaultMaxLines, "maximum lines per file (loc)")
	c.Flags().Float64Var(&maxAvg, "max-comment-avg", lint.DefaultMaxCommentAvg, "maximum mean lines per comment block (comment-length)")
	c.Flags().StringArrayVar(&ignore, "ignore", nil, "skip files matching this glob (no '/' = basename anywhere) or 're:'-prefixed regexp; repeatable")
	return c
}

// repoLintBar resolves the quality bar: the repo's `lint:` block from .sindri/config.yaml,
// unless the caller passed the flag explicitly. An unreadable config falls back to the flag
// defaults — brokkr is a linter, not the arbiter of a project's config, and the hub reports a
// broken one already.
func repoLintBar(cmd *cobra.Command, flagLines int, flagAvg float64) (int, float64) {
	lines, avg := flagLines, flagAvg
	cfg, err := config.Load(".")
	if err != nil {
		return lines, avg
	}
	if cfg.Lint.MaxLines != nil && !cmd.Flags().Changed("max") {
		lines = *cfg.Lint.MaxLines
	}
	if cfg.Lint.MaxCommentAvg != nil && !cmd.Flags().Changed("max-comment-avg") {
		avg = *cfg.Lint.MaxCommentAvg
	}
	return lines, avg
}

// runLinters runs the named linter, or all of them when which is empty. Returns
// whether any violation was found.
func runLinters(out io.Writer, which, tags string, maxLines int, maxAvg float64, ig *lint.Ignore) (bool, error) {
	switch which {
	case "deadcode":
		return lint.Deadcode([]string{"./..."}, tags, ig, out)
	case "loc":
		return lint.LOC([]string{"."}, maxLines, ig, out)
	case "comments":
		return runComments(out, ig)
	case "comment-length":
		return lint.CommentAvg([]string{"."}, maxAvg, ig, out)
	case "js":
		return lint.JSLint(".", ig, out)
	case "openspec":
		return lintOpenspec(out), nil
	case "":
		return runAll(out, tags, maxLines, maxAvg, ig)
	default:
		return false, fmt.Errorf("unknown linter %q (want deadcode|loc|comments|comment-length|js|openspec)", which)
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

// runAll runs every linter in turn, streaming each section live, then ends with a
// summary. On failure the summary NAMES the failing linters and re-surfaces their
// findings — so the culprit is obvious even when the live sections scrolled off or
// `--tail` clipped them (the reason you never have to run a single linter to find
// out what failed).
func runAll(out io.Writer, tags string, maxLines int, maxAvg float64, ig *lint.Ignore) (bool, error) {
	linters := []struct {
		name string
		run  func(io.Writer) (bool, error)
	}{
		{"deadcode", func(w io.Writer) (bool, error) { return lint.Deadcode([]string{"./..."}, tags, ig, w) }},
		{"loc", func(w io.Writer) (bool, error) { return lint.LOC([]string{"."}, maxLines, ig, w) }},
		{"comments", func(w io.Writer) (bool, error) { return runComments(w, ig) }},
		{"comment-length", func(w io.Writer) (bool, error) { return lint.CommentAvg([]string{"."}, maxAvg, ig, w) }},
		{"js", func(w io.Writer) (bool, error) { return lint.JSLint(".", ig, w) }},
		{"openspec", func(w io.Writer) (bool, error) { return lintOpenspec(w), nil }},
	}
	var failed []string
	captured := map[string]string{}
	for _, l := range linters {
		var buf strings.Builder
		fmt.Fprintf(out, "== %s ==\n", l.name)
		bad, err := l.run(io.MultiWriter(out, &buf)) // stream live AND capture for the recap
		if err != nil {
			// One linter breaking does not cancel the others: they are independent checks, and
			// the findings of the rest are exactly what you need while you fix this one. The
			// error counts as that linter FAILING, so the gate still goes red.
			fmt.Fprintf(io.MultiWriter(out, &buf), "%s: %v\n", l.name, err)
			bad = true
		}
		fmt.Fprintln(out)
		if bad {
			failed = append(failed, l.name)
			captured[l.name] = strings.TrimSpace(buf.String())
		}
	}
	if len(failed) == 0 {
		fmt.Fprintln(out, "OK: all linters passed")
		return false, nil
	}
	fmt.Fprintf(out, "FAIL: %s\n", strings.Join(failed, ", "))
	for _, name := range failed {
		if c := captured[name]; c != "" {
			fmt.Fprintf(out, "\n--- %s ---\n%s\n", name, c)
		}
	}
	return true, nil
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
//
// It delegates to spec.Validate — the same function `sindri openspec submit` gates on — and
// names it in the output (spec.ValidatorName), so a caller can trust one verdict for both.
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
