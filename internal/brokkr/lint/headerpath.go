// package: lint / headerpath
// type:    logic (a file's header names where the file actually is)
// job:     report every `// package: <path> / <name>` header whose path is not the file's own
// directory, so a package that moves cannot leave `brokkr map` describing the old tree.
// limits:  the path segment. Whether the job and limits lines are TRUE is nobody's linter to
// judge — this only checks the one field a move invalidates mechanically.
package lint

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// headerPrefix opens the four-field block brokkr map reads.
const headerPrefix = "// package: "

// HeaderPath reports every source file whose header names a directory other than its own. The
// header's path is written relative to internal/ or cmd/ — `hub/api/serve`, `ui/tui` — so the check
// is that the file's directory ends with it.
func HeaderPath(roots []string, cap *Cap, ig *Ignore, w io.Writer) (bool, error) {
	if len(roots) == 0 {
		roots = []string{"."}
	}
	var bad []string
	seen := 0
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
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || ig.Match(path) {
				return nil
			}
			declared, ok, rerr := headerOf(path)
			if rerr != nil {
				return rerr
			}
			if !ok {
				return nil // a missing header is the comments linter's finding, not this one's
			}
			seen++
			// An entrypoint names itself `main (<binary>)` rather than its directory: there is one
			// package main per binary and the binary is what a reader is looking for.
			if strings.HasPrefix(declared, "main (") {
				return nil
			}
			dir := filepath.ToSlash(filepath.Dir(path))
			if !strings.HasSuffix(dir, declared) {
				bad = append(bad, fmt.Sprintf("%s: its header says `package: %s`, but the file is in %s — "+
					"`brokkr map` prints that header, so a stale one describes a tree that no longer exists",
					filepath.ToSlash(path), declared, dir))
			}
			return nil
		})
		if err != nil {
			return false, err
		}
	}
	if seen < 50 {
		return false, fmt.Errorf("only %d headers read — the scan is not reading the tree", seen)
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
		fmt.Fprintf(w, "%d header(s) name the wrong directory. The path segment is written relative to "+
			"internal/ or cmd/ — a package that moves updates it in the same commit.\n", len(bad))
	}
	return len(bad) > 0, nil
}

// headerOf is the path segment of a file's `// package: <path> / <name>` header, ok=false when the
// file opens with no such header.
func headerOf(path string) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "package ") {
			return "", false, nil // reached the clause with no header above it
		}
		rest, found := strings.CutPrefix(line, headerPrefix)
		if !found {
			continue
		}
		declared, _, _ := strings.Cut(rest, " / ")
		return strings.TrimSpace(declared), true, sc.Err()
	}
	return "", false, sc.Err()
}
