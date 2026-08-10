// package: arch / vocab
// type:    test (architecture invariant)
// job:     fail the build if any non-test .go file declares a header `type:` outside
// the seven the architecture spec allows — logic, adapter, assembly, rendering,
// ui, command, entrypoint. Same home and reasoning as the front-end import
// guard (-> internal/ui/importguard_test.go): this project's own rule, in this
// project's own test, not folded into brokkr's generic toolbelt.
// limits:  the enum only; `brokkr lint comments` already checks the four header
// fields are present at all.
package arch

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// legalTypes are the architecture spec's closed set (01-architecture, "Layer types and their rules").
var legalTypes = map[string]bool{
	"logic": true, "adapter": true, "assembly": true, "rendering": true,
	"ui": true, "command": true, "entrypoint": true,
}

// vocabSkipDirs mirrors brokkr lint's skip list: code this project did not write, or output it did
// not hand-author, never carries this project's header convention.
var vocabSkipDirs = map[string]bool{
	".git": true, ".worktrees": true, ".sindri": true, ".todos": true,
	"vendor": true, "node_modules": true,
}

// typeLine matches the header's `type:` field; the value is everything up to the first space or
// end of line, so a parenthetical note after it (e.g. "adapter (SQLite, hub-owned)") is dropped.
var typeLine = regexp.MustCompile(`^//\s*type:\s*(\S+)`)

// moduleRoot locates the repo root via `go env GOMOD` rather than `git`, which this sandbox's
// worktree setup does not always expose to a subprocess the way plain Go tooling is.
func moduleRoot(t *testing.T) string {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == string(filepath.Separator)+"dev"+string(filepath.Separator)+"null" {
		t.Fatal("go env GOMOD returned no module — this test must run inside the module")
	}
	return filepath.Dir(gomod)
}

// headerType reads a file's header `type:` value from its first 8 lines (the header always opens
// the file, per the architecture spec); ok=false if none is found.
func headerType(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 8 {
		lines = lines[:8]
	}
	for _, line := range lines {
		if m := typeLine.FindStringSubmatch(line); m != nil {
			return m[1], true
		}
	}
	return "", false
}

// TestLayerVocabularyIsAClosedSet walks every non-test .go file in the module and fails on any
// header `type:` value outside the seven the architecture spec enumerates — the enforcement the
// convention itself has none of, since `brokkr lint comments` checks the fields are present, not
// what they say.
func TestLayerVocabularyIsAClosedSet(t *testing.T) {
	root := moduleRoot(t)
	var bad []string
	seen := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if vocabSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		seen++
		rel, _ := filepath.Rel(root, path)
		typ, ok := headerType(path)
		if !ok {
			bad = append(bad, fmt.Sprintf("%s: no header `type:` field found", rel))
			return nil
		}
		if !legalTypes[typ] {
			bad = append(bad, fmt.Sprintf("%s: type %q is not one of the seven legal values (logic, adapter, assembly, rendering, ui, command, entrypoint)", rel, typ))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	// Prove the walk actually found files, the way the import guard proves its graph was read —
	// an empty tree would otherwise pass this test by finding nothing to fail on.
	if seen == 0 {
		t.Fatalf("walked %s and found no non-test .go files — the guard is not looking at the repo", root)
	}
	sort.Strings(bad)
	for _, msg := range bad {
		t.Error(msg)
	}
}
