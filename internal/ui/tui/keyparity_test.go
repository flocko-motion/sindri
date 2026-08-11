package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// declaredSingleKeys is every single-rune key any keymap row names, regardless of scope — the
// inverse of scopeLabels' per-scope view, since this guard asks a simpler question: is the key
// declared ANYWHERE, not what it means on a given tab (TestNoTwoActionsShareAKeyOnATab already
// owns that question).
func declaredSingleKeys() map[string]bool {
	out := map[string]bool{}
	for _, b := range keymap {
		for _, part := range splitKeys(b.keys) {
			if len([]rune(part)) == 1 {
				out[part] = true
			}
		}
	}
	return out
}

// splitKeys is strings.Split(s, "/") without importing strings twice for one call — matches
// scopeLabels' own splitting convention exactly.
func splitKeys(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if r == '/' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// handledLetterKeys parses onkey.go's switch statements and keys.go's key constants, and returns
// every single-ASCII-letter key onKey actually dispatches on — a `case "j":` literal, or a
// `case keyPriority:` whose constant resolves to one letter. Multi-character dispatch strings
// ("ctrl+d", "tab", "enter", "shift+tab", ...) are deliberately excluded: the keymap's own
// single-rune convention (scopeLabels) never claims to cover those, so this guard doesn't either —
// it only makes real the promise the existing convention already implies for plain letter keys.
func handledLetterKeys(t *testing.T) map[string]bool {
	t.Helper()
	consts := keyConstants(t)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "onkey.go", nil, 0)
	if err != nil {
		t.Fatalf("parse onkey.go: %v", err)
	}
	out := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		sw, ok := n.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		for _, stmt := range sw.Body.List {
			cc, ok := stmt.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, expr := range cc.List {
				if lit, ok := singleLetter(expr, consts); ok {
					out[lit] = true
				}
			}
		}
		return true
	})
	return out
}

// singleLetter resolves a case expression (a string literal, or an identifier naming a key
// constant) to a single ASCII letter, ok=false for anything else (multi-char strings, non-key
// identifiers, unrecognized expression shapes).
func singleLetter(expr ast.Expr, consts map[string]string) (string, bool) {
	var raw string
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		raw = e.Value[1 : len(e.Value)-1] // strip the surrounding quotes
	case *ast.Ident:
		v, ok := consts[e.Name]
		if !ok {
			return "", false
		}
		raw = v
	default:
		return "", false
	}
	r := []rune(raw)
	if len(r) != 1 {
		return "", false
	}
	c := r[0]
	if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
		return raw, true
	}
	return "", false
}

// keyConstants parses keys.go's const block into name -> literal value, so a `case keyPriority:`
// resolves the same way a reader of the source would.
func keyConstants(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "keys.go", nil, 0)
	if err != nil {
		t.Fatalf("parse keys.go: %v", err)
	}
	out := map[string]string{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != len(vs.Values) {
				continue
			}
			for i, name := range vs.Names {
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				out[name.Name] = lit.Value[1 : len(lit.Value)-1]
			}
		}
	}
	return out
}

// TestEveryHandledLetterKeyIsInTheKeymap is the inverse of TestNoTwoActionsShareAKeyOnATab: every
// plain-letter key onKey actually dispatches on must be declared in keymap somewhere, or it is a
// real binding nobody can discover — which is exactly how J/K/g/G/y/Y/h/l accumulated unadvertised
// before this test existed. A binding added to onKey without a matching keymap row now fails the
// build instead of quietly shipping invisible.
func TestEveryHandledLetterKeyIsInTheKeymap(t *testing.T) {
	declared := declaredSingleKeys()
	for key := range handledLetterKeys(t) {
		if !declared[key] {
			t.Errorf("onKey handles %q but no keymap row declares it — add a binding entry", key)
		}
	}
}
