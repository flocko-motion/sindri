package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOnlyTheOwnedSourceWritesOwnedStatus is the rule the recurring bug kept breaking: where a
// task's status lives is the SOURCE's business, and nothing else may know. Four separate places
// grew an "if ps.OwnsTask" of their own, and the three that forgot each left an openspec task open
// over finished work — a caller that has to know the kind is one that will eventually get it wrong.
//
// So SetOwnedStatus belongs to ownedsource.go, which IS the td- source, and to SetStatus, which is
// the one verb callers use. Anywhere else is a caller branching on kind again.
func TestOnlyTheOwnedSourceWritesOwnedStatus(t *testing.T) {
	allowed := map[string]string{
		"ownedsource.go": "the td- source itself — writing owned_tasks is precisely its job",
		"close.go":       "SetStatus, the one verb that knows where each kind's status lives",
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		checked++
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), "SetOwnedStatus(") {
			continue
		}
		if _, ok := allowed[f]; !ok {
			t.Errorf("%s writes owned_tasks directly — use Engine.SetStatus, which knows where a "+
				"task's status lives so callers need not", f)
		}
	}
	if checked < 5 {
		t.Fatalf("only %d files scanned — the guard is not reading the package", checked)
	}
}
