// package: lint / jslint
// type:    logic (delegating to the JS/TS toolchain)
// job:     run each JS/TS project's OWN checks (tsc --noEmit, eslint) when configured.
// Reports whether it WAS ABLE to check: a tool missing skips, a config missing
// is still a finding.
// limits:  shells out and relays; brokkr's own rules (headers, length, comment trend) are the
// other files' work.
package lint

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// jsToolTimeout bounds one delegated tool; past it the run is inconclusive, not a hung gate.
const jsToolTimeout = 5 * time.Minute

// eslintConfigs and tsConfigs are the filenames that mean "this project configures that tool".
var (
	eslintConfigs = []string{
		"eslint.config.js", "eslint.config.mjs", "eslint.config.cjs", "eslint.config.ts",
		".eslintrc", ".eslintrc.js", ".eslintrc.cjs", ".eslintrc.json", ".eslintrc.yml", ".eslintrc.yaml",
	}
	tsConfigs = []string{"tsconfig.json"}
)

// JSLint runs each JS/TS project's OWN checks, so brokkr never disagrees with the editor and CI.
// Source with NO tool configured above it is still a finding — that is a project decision the
// project can act on, unlike a configured tool that just isn't installed here.
func JSLint(root string, ig *Ignore, w io.Writer) (bool, error) {
	if root == "" {
		root = "."
	}
	sources, projects, err := scanJSTree(root, ig)
	if err != nil {
		return false, err
	}
	if len(sources) == 0 {
		return false, nil // no JS/TS source — nothing to say
	}

	// Attribute every source file to the project directory nearest above it.
	covered := map[string]int{}
	orphans := map[string]int{}
	for _, src := range sources {
		if owner, ok := nearestProject(src, projects); ok {
			covered[owner]++
			continue
		}
		orphans[topDir(root, src)]++
	}

	failed := false
	for dir, n := range orphans {
		fmt.Fprintf(w, "js: %d file(s) under %s with no tsconfig.json and no eslint config above them — "+
			"the type checker and linter that would catch real defects are not running.\n"+
			"    Add a tsconfig.json (`tsc --init`) and/or an eslint config, or exclude the tree in %s.\n",
			n, dir, IgnoreFileName)
		failed = true
	}

	for _, dir := range sortedKeys(covered) {
		hasTS := firstPresent(dir, tsConfigs) != ""
		hasESLint := firstPresent(dir, eslintConfigs) != ""
		if !hasTS && !hasESLint {
			// A package.json alone: the project is declared but nothing checks it.
			fmt.Fprintf(w, "js: %s has %d file(s) and a package.json, but no tsconfig.json and no eslint config.\n",
				dir, covered[dir])
			failed = true
			continue
		}
		if hasTS && runJSTool(dir, w, "type-check "+dir, "tsc", "--noEmit", "--pretty", "false") {
			failed = true
		}
		if hasESLint && runJSTool(dir, w, "eslint "+dir, "eslint", ".") {
			failed = true
		}
	}
	return failed, nil
}

// scanJSTree walks root once for both the JS/TS sources and the dirs that declare a project.
func scanJSTree(root string, ig *Ignore) (sources []string, projects map[string]bool, err error) {
	projects = map[string]bool{}
	markers := append(append([]string{"package.json"}, tsConfigs...), eslintConfigs...)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		for _, m := range markers {
			if d.Name() == m {
				projects[filepath.Dir(path)] = true
				break
			}
		}
		if LangOf(path) == LangTS && !ig.Match(path) {
			sources = append(sources, path)
		}
		return nil
	})
	return sources, projects, err
}

// nearestProject is the project directory closest above path — the config a compiler would
// resolve for that file.
func nearestProject(path string, projects map[string]bool) (string, bool) {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if projects[dir] {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
	}
}

// topDir is the first path element of src below root, so an orphan report names the tree rather
// than every file in it.
func topDir(root, src string) string {
	rel, err := filepath.Rel(root, src)
	if err != nil {
		return filepath.Dir(src)
	}
	if i := strings.IndexRune(rel, filepath.Separator); i > 0 {
		return rel[:i]
	}
	return "."
}

// sortedKeys gives a map's keys in order, so a report reads the same on every run.
func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// runJSTool runs one delegated tool and relays its output. Configured but not installed is a skip,
// like every other optional delegate — a gate tree is a fresh checkout with node_modules gitignored,
// so a project's own tool is never there to install into it.
func runJSTool(root string, w io.Writer, label string, bin string, args ...string) bool {
	cmd, how := jsCommand(root, bin, args...)
	if cmd == nil {
		fmt.Fprintf(w, "js/%s: %s configured but not installed — skipping (optional)\n", label, bin)
		fmt.Fprintf(w, "    hint: %s\n", missingToolHint(bin))
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), jsToolTimeout)
	defer cancel()
	run := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	run.Dir = root
	out, err := run.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if ctx.Err() != nil {
		fmt.Fprintf(w, "js/%s: timed out after %s (%s) — inconclusive, not a pass.\n", label, jsToolTimeout, how)
		return true
	}
	if err == nil {
		return false
	}
	fmt.Fprintf(w, "js/%s: failed (%s)\n", label, how)
	if text != "" {
		fmt.Fprintln(w, indentLines(text, "    "))
		return true
	}
	// No output means the tool never spoke, and only err says why. A bare "failed" sent a
	// reader hunting a type error when the fault was the exec itself.
	fmt.Fprintf(w, "    no output — the tool did not run: %v\n", err)
	return true
}

// toolPackage is the npm package that provides a binary, and toolAction what running it checks —
// together they name what a hint says would start working.
var (
	toolPackage = map[string]string{"tsc": "typescript", "eslint": "eslint"}
	toolAction  = map[string]string{"tsc": "the type-check", "eslint": "the lint"}
)

// missingToolHint says what installing would BUY, not what to run: the same words as an imperative
// sent dvalin looking for a way to install into a gate tree that gets thrown away every run. Only
// two cases, not the three missingToolAdvice used to probe: with node present, "no node_modules"
// and "node_modules without the binary" both buy the same gain, so they collapse into one line.
func missingToolHint(bin string) string {
	pkg := toolPackage[bin]
	if pkg == "" {
		pkg = bin
	}
	action := toolAction[bin]
	if action == "" {
		action = "the check"
	}
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Sprintf("installing Node.js and %s here would let %s run.", pkg, action)
	}
	return fmt.Sprintf("install %s here and %s runs.", pkg, action)
}

// jsCommand prefers the repo's pinned tool, then a global one, nil if neither. Never `npx`: on the
// registry `tsc` is an unrelated impostor, so it would run something that is not the compiler.
func jsCommand(root, bin string, args ...string) (argv []string, how string) {
	local := filepath.Join(root, "node_modules", ".bin", bin)
	if _, err := os.Stat(local); err == nil {
		// ABSOLUTE: the child resolves a relative argv[0] AFTER chdir into Dir, so
		// "web/node_modules/.bin/tsc" became "web/web/…" and never started.
		if abs, err := filepath.Abs(local); err == nil {
			local = abs
		}
		return append([]string{local}, args...), "node_modules/.bin/" + bin
	}
	if p, err := exec.LookPath(bin); err == nil {
		return append([]string{p}, args...), bin + " (global)"
	}
	return nil, ""
}

// firstPresent returns the first of names that exists under root, or "".
func firstPresent(root string, names []string) string {
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(root, n)); err == nil {
			return n
		}
	}
	return ""
}

// indentLines prefixes every line of s, so relayed tool output reads as a nested report.
func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
