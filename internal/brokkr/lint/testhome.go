// package: lint / testhome
// type:    logic (a test file lives beside what it tests)
// job:     report every `_test.go` with no source file it could belong to, so a test cannot go on
// naming a subject that has moved out from under it.
// limits:  names only. WHETHER a test asserts the right thing is nobody's linter to judge, and a
// file that legitimately covers several subjects is still expected to pick one and say so.
package lint

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// goosArch are the suffixes Go reads as a build constraint ON THE FILENAME. A file called
// clear_arm_test.go is compiled only on GOARCH=arm — so it had never run anywhere, on any machine,
// and the assertion inside it was wrong. Nothing announces this: the file is simply skipped.
var goosArch = map[string]bool{
	"aix": true, "android": true, "darwin": true, "dragonfly": true, "freebsd": true, "hurd": true,
	"illumos": true, "ios": true, "js": true, "linux": true, "nacl": true, "netbsd": true,
	"openbsd": true, "plan9": true, "solaris": true, "wasip1": true, "windows": true, "zos": true,
	"386": true, "amd64": true, "arm": true, "arm64": true, "loong64": true, "mips": true,
	"mips64": true, "mips64le": true, "mipsle": true, "ppc64": true, "ppc64le": true, "riscv": true,
	"riscv64": true, "s390x": true, "sparc64": true, "wasm": true,
}

// TestHome reports every test file with no source file of the same stem beside it. `foo_test.go`
// wants `foo.go`; `foo_bar_test.go` will also take `foo.go`, so a large one can be split by aspect
// (`gitcmd_rollback_test.go`) without inventing a source file to justify the name.
//
// The rule catches the failure this repo keeps having: code moves to the package that owns it and
// its tests stay behind, held by a fixture, until a directory is 8:1 tests for subjects it no
// longer contains. A test whose stem names nothing is the first visible symptom.
func TestHome(roots []string, cap *Cap, ig *Ignore, w io.Writer) (bool, error) {
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
			if !strings.HasSuffix(path, "_test.go") || ig.Match(path) {
				return nil
			}
			// A directory of nothing but tests has no source to name — internal/arch is a package of
			// guards over the whole tree, and asking each of them for a foo.go beside it would be
			// asking for a file with nothing to put in it.
			// Ahead of the home check, and reported whatever else is true of the file: a name Go
			// silently skips is worse than one that names nothing, because the tests inside it pass
			// by never running.
			if tail := lastSegment(stemOf(path)); goosArch[tail] {
				bad = append(bad, fmt.Sprintf("%s: Go reads %q as a build constraint, so this file is "+
					"compiled only on that platform and its tests never run here — rename it", path, tail))
				return nil
			}
			if homeFor(path) != "" || testOnlyDir(filepath.Dir(path)) {
				return nil
			}
			bad = append(bad, fmt.Sprintf("%s: no source file it belongs to — want %s beside it, "+
				"or a stem that names one (%s)", path, filepath.Base(stemOf(path))+".go",
				strings.Join(candidates(path), ", ")))
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
		fmt.Fprintf(w, "%d test file(s) name no source. Move the test to the package that holds "+
			"what it exercises, or rename it after a file that is already there — `foo_bar_test.go` "+
			"counts as a split of `foo.go`, so a long one can be cut by aspect.\n", len(bad))
	}
	return len(bad) > 0, nil
}

// lastSegment is the part of a stem after its final underscore, "" when it has none.
func lastSegment(stem string) string {
	base := filepath.Base(stem)
	if cut := strings.LastIndex(base, "_"); cut > 0 {
		return base[cut+1:]
	}
	return ""
}

// stemOf is a test file's path without the _test.go suffix.
func stemOf(path string) string { return strings.TrimSuffix(path, "_test.go") }

// candidates are the source files this test's name could be claiming, longest stem first: the whole
// stem, then each prefix ending at an underscore, so a split file still points at what it splits.
func candidates(path string) []string {
	stem := stemOf(path)
	dir, base := filepath.Split(stem)
	out := []string{filepath.Join(dir, base+".go")}
	for {
		cut := strings.LastIndex(base, "_")
		if cut <= 0 {
			return out
		}
		base = base[:cut]
		out = append(out, filepath.Join(dir, base+".go"))
	}
}

// testOnlyDir reports a directory holding tests and no Go source of its own.
func testOnlyDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
			return false
		}
	}
	return true
}

// homeFor is the first candidate that exists, "" when none does.
func homeFor(path string) string {
	for _, c := range candidates(path) {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}
