// package: lint / loc
// type:    logic
// job:     the file-length linter — walks Go sources under the given roots and
//          reports any file exceeding the line limit.
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
	"strings"
)

// DefaultMaxLines is the per-file limit from the architecture spec.
const DefaultMaxLines = 700

// skipDirs are directories never scanned for source length.
var skipDirs = map[string]bool{
	".git": true, ".worktrees": true, "vendor": true, "node_modules": true,
}

// LOC walks the given roots (default ".") for .go files and reports each file
// whose line count exceeds maxLines. Returns true if any violation was found.
func LOC(roots []string, maxLines int, w io.Writer) (bool, error) {
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
			if !strings.HasSuffix(path, ".go") {
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

	sort.Slice(viols, func(i, j int) bool { return viols[i].lines > viols[j].lines })
	for _, v := range viols {
		fmt.Fprintf(w, "%s: %d lines (limit %d)\n", v.path, v.lines, maxLines)
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
