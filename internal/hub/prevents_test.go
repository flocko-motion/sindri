package hub

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestEveryPREventIsClassified makes the lifecycle summary fail closed. Twenty-two event types are
// logged against a PR and the summary shows a chosen few, so the 23rd would land in whichever
// bucket the default happened to be — hidden if diagnostic (a state nobody can see), noisy if
// milestone (the clutter the summary exists to avoid). Neither failure announces itself, so the
// build says so instead: adding a LogPR type is a violation until it is classified in
// api.prEventMilestone. The keymap's TestLowercaseKeysNeverMutate holds the same line the same way.
func TestEveryPREventIsClassified(t *testing.T) {
	types := loggedPREventTypes(t, "..")
	if len(types) < 20 {
		t.Fatalf("found only %d LogPR types — the scanner is not reading the hub", len(types))
	}
	for _, typ := range types {
		if _, known := api.PREventKind(typ); !known {
			t.Errorf("PR event %q is logged but classified nowhere: say in internal/api whether it "+
				"belongs in the lifecycle summary or the full log", typ)
		}
	}
}

// loggedPREventTypes collects the literal event types passed to any LogPR call under dir — the
// vocabulary as the code actually writes it, rather than a list kept by hand beside it.
func loggedPREventTypes(t *testing.T, dir string) []string {
	t.Helper()
	seen := map[string]bool{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil //nolint:nilerr // an unreadable entry logs nothing; the rest still do
		}
		file, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", path, perr)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "LogPR" || len(call.Args) < 2 {
				return true
			}
			// The type is the second argument; a non-literal one (the store's own wrapper passes a
			// variable) says nothing about the vocabulary and is skipped rather than guessed at.
			if lit, ok := call.Args[1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				seen[strings.Trim(lit.Value, `"`)] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	out := make([]string, 0, len(seen))
	for typ := range seen {
		out = append(out, typ)
	}
	sort.Strings(out)
	return out
}

// TestTheClassificationHasNoStrangers is the other direction: a type classified but never logged is
// dead vocabulary, and a renamed event would otherwise leave its old name sitting there looking
// deliberate.
func TestTheClassificationHasNoStrangers(t *testing.T) {
	logged := map[string]bool{}
	for _, typ := range loggedPREventTypes(t, "..") {
		logged[typ] = true
	}
	for _, typ := range api.PREventTypes() {
		if !logged[typ] {
			t.Errorf("PR event %q is classified but nothing logs it — drop it, or the list stops "+
				"describing what actually happens", typ)
		}
	}
}
