// package: codemap / walk
// type:    logic (tree traversal + scan reporting)
// job:     the ONE walk every codemap search goes through — map, --find, --grep, --symbol and
// refs — and the honest report of what it read, so an empty answer says whether it found
// nothing or read nothing.
// limits:  traversal and the two scan messages; what each file yields is its caller's.
package codemap

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// walkGoFiles calls fn for every .go file under each root, labelled as the map labels files; fn
// reports whether that file yielded anything, so the caller learns how much source was READ and
// how much ANSWERED. maxDepth bounds descent (0 = root only, negative = unlimited); an unwalkable
// root errors before anything is emitted, so a typo in a later one cannot print a half-answer.
func walkGoFiles(roots []string, maxDepth int, fileFilter string, fn func(disp, path string) bool) (visited, matched int, err error) {
	cwd, _ := os.Getwd()
	multi := len(roots) > 1
	for _, root := range roots {
		if _, err := os.Stat(root); err != nil {
			return 0, 0, err
		}
	}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDirs[d.Name()] || (maxDepth >= 0 && dirDepth(root, path) > maxDepth) {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			disp := displayPath(root, cwd, path, multi)
			if fileFilter != "" && !strings.Contains(strings.ToLower(disp), fileFilter) {
				return nil
			}
			visited++
			if fn(disp, path) {
				matched++
			}
			return nil
		})
		if err != nil {
			return visited, matched, err
		}
	}
	return visited, matched, nil
}

// reportScan explains an empty answer and stays quiet when there is a real one. Two emptinesses
// used to look identical: a tree with no Go in it (never read, so not evidence of absence) and one
// read in full with no match. what names the query, so the message can be acted on.
func reportScan(w io.Writer, visited, matched int, roots []string, what string) {
	if matched > 0 {
		return
	}
	where := strings.Join(roots, " ")
	if visited == 0 {
		fmt.Fprintf(w, "no Go files under %s — brokkr map and refs read Go only.\n", where)
		return
	}
	fmt.Fprintf(w, "scanned %s, no match for %s.\n", plural(visited, "Go file"), what)
}

// plural counts a thing in words, so a message never reads "1 files".
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
