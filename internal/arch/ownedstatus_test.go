package arch

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ownedStatusWriters are the only places allowed to write the owned_tasks status column, each with
// why. Anywhere else is a caller branching on what KIND of task it holds, which is the shape that
// keeps recurring: four sites grew their own "if OwnsTask", and the three that forgot each left a
// finished openspec change reading open.
var ownedStatusWriters = map[string]string{
	"internal/hub/store/owned.go":       "the column's own accessor",
	"internal/hub/owned/owned.go":       "the source that OWNS these rows — writing them is its job",
	"internal/hub/flow/pr/close_act.go": "SetStatus, the one verb that knows where each kind's status lives",
}

// TestOnlyTheOwnedSourceWritesOwnedStatus walks the WHOLE tree, which is the point: the same guard
// scoped to one package passed for months while a fourth writer sat in hub/agent, because a glob of
// "*.go" only ever sees the package it lives in.
func TestOnlyTheOwnedSourceWritesOwnedStatus(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "internal")
	seen := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return walkErr
		}
		seen++
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if !strings.Contains(string(src), "SetOwnedStatus(") {
			return nil
		}
		rel, _ := filepath.Rel(moduleRoot(t), path)
		if _, ok := ownedStatusWriters[filepath.ToSlash(rel)]; !ok {
			t.Errorf("%s writes owned_tasks directly — ask SetStatus instead, which knows where a "+
				"task's status lives so callers need not know its kind", filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if seen < 100 {
		t.Fatalf("only %d files scanned — the guard is not reading the tree", seen)
	}
}
