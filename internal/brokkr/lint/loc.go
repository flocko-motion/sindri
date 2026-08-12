// package: lint / loc
// type:    logic
// job:     the file-length linter — walks every linted source under the given roots (Go and
// the TypeScript/JavaScript family) and reports any file exceeding the line limit.
// limits:  reports only; the CLI wiring and exit codes live in cmd/sindri/lint.go.
package lint

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// DefaultMaxLines is the per-file limit from the architecture spec.
const DefaultMaxLines = 700

// skipDirs are directories never scanned: code the project did not write, and output it did
// not hand-author. `.sindri` holds the hub's per-agent homes, which carry whole vendored plugin
// trees — third-party TypeScript that would otherwise be reported as this project's own.
var skipDirs = map[string]bool{
	".git": true, ".worktrees": true, ".sindri": true, ".todos": true,
	"vendor": true, "node_modules": true,
	// JS/TS build output and coverage: generated, and often shipped in-tree.
	"dist": true, "build": true, "out": true, ".next": true, ".nuxt": true,
	".svelte-kit": true, "coverage": true, ".turbo": true,
}

// LOC walks the given roots (default ".") for .go files and reports each file
// whose line count exceeds maxLines, skipping any path matched by ig. Returns
// true if any violation was found.
func LOC(roots []string, maxLines int, cap *Cap, ig *Ignore, w io.Writer) (bool, error) {
	if len(roots) == 0 {
		roots = []string{"."}
	}
	if maxLines <= 0 {
		maxLines = DefaultMaxLines
	}

	type viol struct {
		path  string
		lines int
	}
	var viols []viol

	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			// Shell is linted for comment length only. It is excluded here by name rather than by
			// falling out of LangOf, because this filter is written as "any language we know" —
			// so every language added to Lang joins this rule silently unless it says otherwise.
			if l := LangOf(path); l == LangNone || l == LangShell || ig.Match(path) {
				return nil
			}
			n, err := countLines(path)
			if err != nil {
				return err
			}
			if n > maxLines {
				viols = append(viols, viol{path, n})
			}
			return nil
		})
		if err != nil {
			return false, err
		}
	}

	// Longest first, so a capped run withholds the files nearest the limit.
	sort.Slice(viols, func(i, j int) bool { return viols[i].lines > viols[j].lines })
	for _, v := range viols {
		if !cap.Allow() {
			continue
		}
		// Say how far over AND what to do. "721 lines (limit 700)" reads as "delete 21 lines",
		// which is the wrong fix: the file is over because it holds more than one job, and
		// shaving it lands on the limit until the next function arrives.
		fmt.Fprintf(w, "%s: %d lines (max %d, %d over) — SPLIT it, don't shave it: move a "+
			"cohesive group out to its own file.\n", v.path, v.lines, maxLines, v.lines-maxLines)
	}
	cap.Note(w)
	if len(viols) > 0 {
		fmt.Fprintf(w, "%d file(s) over %d lines. A file this long is doing more than one job — "+
			"`brokkr map <file>` shows its declarations; lift the biggest cohesive group into a new "+
			"file with its own four-field header. Trimming to just under the limit is not a fix.\n",
			len(viols), maxLines)
	}
	return len(viols) > 0, nil
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		n++
	}
	return n, sc.Err()
}
