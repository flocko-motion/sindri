// package: lint / gofmt
// type:    logic (formatting)
// job:     report Go files gofmt would rewrite, so formatting drift fails the gate instead of
// arriving later as a file someone's editor mangled on save.
// limits:  reports only, never rewrites — `gofmt -w` is the fix. Uses go/format, so it needs no
// gofmt binary on PATH.
package lint

import (
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
)

// Gofmt reports every Go file whose formatting differs from gofmt's, naming the first line that
// differs so the cause is findable without diffing by hand.
//
// It runs through go/format rather than the gofmt binary: the toolchain is optional here (deadcode
// skips without it), and formatting should not be.
func Gofmt(roots []string, cap *Cap, ig *Ignore, w io.Writer) (bool, error) {
	if len(roots) == 0 {
		roots = []string{"."}
	}
	var bad []string
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
			// Test files count: formatting is not a documentation rule.
			if LangOf(path) != LangGo || ig.Match(path) {
				return nil
			}
			src, ok := readSource(path)
			if !ok {
				return nil
			}
			want, ferr := format.Source([]byte(src))
			if ferr != nil {
				return nil // unparseable: that is the compiler's complaint, not this one's
			}
			if string(want) == src {
				return nil
			}
			bad = append(bad, fmt.Sprintf("%s:%d: not gofmt-clean — run `gofmt -w %s`",
				path, firstDiffLine(src, string(want)), path))
			return nil
		})
		if err != nil {
			return false, err
		}
	}
	for _, msg := range bad {
		if !cap.Allow() {
			continue
		}
		fmt.Fprintln(w, msg)
	}
	if len(bad) > 0 {
		cap.Note(w)
		if cap.Quiet() {
			return true, nil
		}
		fmt.Fprintf(w, "%d file(s) gofmt would rewrite. In this repo the usual cause is a comment "+
			"line indented under the one above it: gofmt reads that as a code block and replaces it "+
			"with a tab plus blank `//` separators, which destroys a four-field header. Wrapped "+
			"comment lines start at column 3.\n", len(bad))
	}
	return len(bad) > 0, nil
}

// firstDiffLine is the 1-based line where got and want part company, or 1 when they differ only in
// length. A bare "not formatted" sends you diffing the whole file.
func firstDiffLine(got, want string) int {
	g, v := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := range g {
		if i >= len(v) {
			return i + 1
		}
		if g[i] != v[i] {
			return i + 1
		}
	}
	return len(g)
}
