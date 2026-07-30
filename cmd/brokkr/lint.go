// package: main (brokkr) / lint
// type:    command
// job:     wires `brokkr lint` — no argument runs every linter (deadcode, loc,
// comments, openspec) with a summary; `brokkr lint <name>` runs one.
// Exits non-zero on any violation, so it can gate CI; add --tail to also
// print the exit status inline.
// limits:  Go analyses live in internal/lint, openspec validation in
// adapter/spec; this only wires flags, dispatch, and exit codes.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	var maxLine int
	var blocks bool
	var limit int
	var ignore []string
	c := &cobra.Command{
		Use:   "lint [linter] [paths...]",
		Short: "Run the quality gate: lint (all) or lint <deadcode|loc|comments|comment-length|js|openspec> [paths]",
		Long: "Run the project's static-analysis linters. With no argument, runs them " +
			"all (deadcode, loc, comments, comment-length, js, openspec) with a summary; with a linter " +
			"name, runs just that one. Exits non-zero on any violation, so it can gate " +
			"CI (add --tail to also print the exit status inline).\n\n" +
			"Trailing paths scope the run to those files or directories, so you can work on one " +
			"file instead of grepping a whole-repo report: `brokkr lint comment-length " +
			"internal/hub/state.go`. A first argument that isn't a linter name is read as a path, " +
			"so `brokkr lint internal/hub` runs every linter over that tree.\n\n" +
			"For comment-length, --blocks lists every comment over the limit with its line range, " +
			"length and opening words, plus how many comment lines have to go — so a file is one " +
			"edit rather than a read-guess-recheck loop.\n\n" +
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
		Args: cobra.ArbitraryArgs,
		// Failures report themselves via exitCodeError, so cobra must not echo them too.
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			which, paths := splitLintArgs(args)
			// The checked-in .brokkrignore, for generated files that can't carry a marker.
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
			o := lintOpts{
				tags: tags, maxLines: lines, maxAvg: avg, maxLine: maxLine,
				blocks: blocks, paths: paths, ig: ig,
				// ONE budget for the whole run: six linters each printing "only" their share is
				// the wall this bounds. It is why `brokkr lint` needs no --tail.
				cap: lint.NewCap(limit),
			}
			return runLint(out, func() (bool, error) { return runLinters(out, which, o) })
		},
	}
	c.Flags().StringVar(&tags, "tags", "", "comma-separated list of extra build tags (deadcode)")
	c.Flags().IntVar(&maxLines, "max", lint.DefaultMaxLines, "maximum lines per file (loc)")
	c.Flags().Float64Var(&maxAvg, "max-comment-avg", lint.DefaultMaxCommentAvg, "maximum mean lines per comment block (comment-length)")
	c.Flags().IntVar(&maxLine, "max-comment-line", lint.DefaultMaxCommentLine, "maximum width of one comment line (comment-length)")
	c.Flags().BoolVar(&blocks, "blocks", false, "list every comment over the limit — line range, length, excerpt (comment-length)")
	c.Flags().IntVar(&limit, "limit", lint.DefaultLimit, "stop after this many findings, then say how many were withheld (0 = all)")
	c.Flags().StringArrayVar(&ignore, "ignore", nil, "skip files matching this glob (no '/' = basename anywhere) or 're:'-prefixed regexp; repeatable")
	return c
}

// lintNames are the linters, and what tells a name from a path in the arguments.
var lintNames = map[string]bool{
	"deadcode": true, "loc": true, "comments": true,
	"comment-length": true, "js": true, "openspec": true,
}

// splitLintArgs reads an optional linter name then paths, so scoping needs no flag or placeholder.
func splitLintArgs(args []string) (which string, paths []string) {
	if len(args) > 0 && lintNames[args[0]] {
		return args[0], args[1:]
	}
	return "", args
}

// orDot defaults a path list to the whole tree, so every linter keeps its unscoped behaviour.
func orDot(paths []string) []string {
	if len(paths) == 0 {
		return []string{"."}
	}
	return paths
}

// pkgPatterns turns paths into package patterns for deadcode; a file scopes to its directory.
func pkgPatterns(paths []string) []string {
	if len(paths) == 0 {
		return []string{"./..."}
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if filepath.Ext(p) != "" {
			p = filepath.Dir(p)
		}
		out = append(out, "./"+filepath.ToSlash(filepath.Clean(p))+"/...")
	}
	return out
}

// repoLintBar reads the repo's `lint:` config unless a flag overrides it; an unreadable one defaults.
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

// lintOpts is one run's resolved configuration. A struct because the thresholds outgrew being
// readable as positional arguments — eleven in a row is how a caller passes the wrong one.
type lintOpts struct {
	tags     string
	maxLines int
	maxAvg   float64
	maxLine  int
	blocks   bool
	paths    []string
	cap      *lint.Cap
	ig       *lint.Ignore
}

// runLinters runs the named linter, or all of them when which is empty, scoped to o.paths.
func runLinters(out io.Writer, which string, o lintOpts) (bool, error) {
	switch which {
	case "deadcode":
		return lint.Deadcode(pkgPatterns(o.paths), o.tags, o.cap, o.ig, out)
	case "loc":
		return lint.LOC(orDot(o.paths), o.maxLines, o.cap, o.ig, out)
	case "comments":
		return runComments(out, o)
	case "comment-length":
		return lint.CommentAvg(orDot(o.paths), o.maxAvg, o.maxLine, o.blocks, o.cap, o.ig, out)
	case "js":
		return runJS(out, o)
	case "openspec":
		return lintOpenspec(out), nil
	case "":
		return runAll(out, o)
	default:
		return false, fmt.Errorf("unknown linter %q (want deadcode|loc|comments|comment-length|js|openspec)", which)
	}
}

// runJS runs the delegated checks per scoped path — JSLint takes one root, so findings are OR'd.
func runJS(out io.Writer, o lintOpts) (bool, error) {
	found := false
	for _, root := range orDot(o.paths) {
		bad, err := lint.JSLint(root, o.ig, out)
		if err != nil {
			return found, err
		}
		found = found || bad
	}
	return found, nil
}

// runComments follows a violation with the convention, so the fix needs no trip elsewhere.
func runComments(out io.Writer, o lintOpts) (bool, error) {
	found, err := lint.Comments(orDot(o.paths), o.cap, o.ig, out)
	if err != nil {
		return false, err
	}
	if found {
		fmt.Fprintf(out, "\n%s\n", commentsConvention)
	}
	return found, nil
}

// runAll runs every linter in its own section and ends by NAMING the failures. It used to re-print
// their findings underneath so they survived --tail, which doubled every run's output — the wall
// `--limit` exists to prevent.
func runAll(out io.Writer, o lintOpts) (bool, error) {
	linters := []struct {
		name string
		run  func(io.Writer) (bool, error)
	}{
		{"deadcode", func(w io.Writer) (bool, error) { return lint.Deadcode(pkgPatterns(o.paths), o.tags, o.cap, o.ig, w) }},
		{"loc", func(w io.Writer) (bool, error) { return lint.LOC(orDot(o.paths), o.maxLines, o.cap, o.ig, w) }},
		{"comments", func(w io.Writer) (bool, error) { return runComments(w, o) }},
		{"comment-length", func(w io.Writer) (bool, error) {
			return lint.CommentAvg(orDot(o.paths), o.maxAvg, o.maxLine, o.blocks, o.cap, o.ig, w)
		}},
		{"js", func(w io.Writer) (bool, error) { return runJS(w, o) }},
		{"openspec", func(w io.Writer) (bool, error) { return lintOpenspec(w), nil }},
	}
	var failed []string
	for _, l := range linters {
		fmt.Fprintf(out, "== %s ==\n", l.name)
		bad, err := l.run(out)
		if err != nil {
			// One linter breaking does not cancel the others — the rest of the findings are what
			// you want while fixing this one. It still counts as FAILING, so the gate goes red.
			fmt.Fprintf(out, "%s: %v\n", l.name, err)
			bad = true
		}
		fmt.Fprintln(out)
		if bad {
			failed = append(failed, l.name)
		}
	}
	if len(failed) == 0 {
		fmt.Fprintln(out, "OK: all linters passed")
		return false, nil
	}
	fmt.Fprintf(out, "FAIL: %s — findings are in the section(s) above.\n", strings.Join(failed, ", "))
	return true, nil
}

// runLint runs one linter's body, turning a panic into a loud failure with its stack. Failure
// returns an exitCodeError, never os.Exit — that would bypass the --tail flush.
func runLint(out io.Writer, fn func() (bool, error)) error {
	if code := lintOutcome(out, fn); code != 0 {
		return exitCodeError{code}
	}
	return nil
}

// lintOutcome runs fn under a panic recover, reports why it failed, and returns the exit code.
// Split from runLint so it is testable in isolation.
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

// commentsConvention explains what the comments linter expects, shown in --help and after a
// violation so the fix needs no trip elsewhere.
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

// lintOpenspec validates the project's specs, a no-op when openspec isn't used or installed. It
// delegates to spec.Validate and names it, so this verdict is the submit gate's verdict.
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
