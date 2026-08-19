package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// directCommitPairs derives, from onkey.go's source, which (key, tab) pairs reach the client
// directly on the bare keystroke — a call to mutateThenRefresh or m.action() found in a case's
// `if m.tab == N { ... }` block WITHOUT passing through a nested func literal, since a call inside
// one is deferred to a confirm/apply and the keystroke itself commits nothing. These are the
// pairs classification cannot get wrong — a confirm-gated action's "which keys move behind the
// prefix to save footer space" is a judgement this cannot derive (sd-6d0ff2's own words).
func directCommitPairs(t *testing.T) map[string][]int {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "onkey.go", nil, 0)
	if err != nil {
		t.Fatalf("parse onkey.go: %v", err)
	}
	consts := keyConstants(t)
	out := map[string][]int{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "onKey" {
			continue
		}
		for _, stmt := range fn.Body.List {
			sw, ok := stmt.(*ast.SwitchStmt)
			if !ok {
				continue
			}
			for _, item := range sw.Body.List {
				cc, ok := item.(*ast.CaseClause)
				if !ok {
					continue
				}
				keys := caseKeyLetters(cc, consts)
				if len(keys) == 0 {
					continue
				}
				for _, tab := range tabsWithDirectClientCall(cc.Body) {
					for _, k := range keys {
						out[k] = append(out[k], tab)
					}
				}
			}
		}
	}
	return out
}

// caseKeyLetters resolves a case clause's labels to the single-letter keys they match.
func caseKeyLetters(cc *ast.CaseClause, consts map[string]string) []string {
	var out []string
	for _, expr := range cc.List {
		if k, ok := singleLetter(expr, consts); ok {
			out = append(out, k)
		}
	}
	return out
}

// tabsWithDirectClientCall finds every `if m.tab == N { ... }` in body whose block commits
// directly on the keystroke, following each else-if chain — onkey.go's multi-tab cases are almost
// always written that way (keyFilter, keyNew, keyTell among them), not as separate top-level ifs,
// and a check that stopped at the first branch would never look at the rest.
func tabsWithDirectClientCall(body []ast.Stmt) []int {
	var tabs []int
	for _, stmt := range body {
		if ifs, ok := stmt.(*ast.IfStmt); ok {
			tabs = append(tabs, tabsInChain(ifs)...)
		}
	}
	return tabs
}

// tabsInChain walks one if / else-if / else-if... chain top to bottom (Go nests each "else if" as
// the previous IfStmt's Else), applying the same per-branch check at every link.
func tabsInChain(ifs *ast.IfStmt) []int {
	var tabs []int
	for ifs != nil {
		if n, ok := tabEquals(ifs.Cond); ok && hasDirectClientCall(ifs.Body) {
			tabs = append(tabs, n)
		}
		next, ok := ifs.Else.(*ast.IfStmt)
		if !ok {
			break
		}
		ifs = next
	}
	return tabs
}

// tabEquals extracts N from a condition shaped like `m.tab == N`, or `m.tab == N && ...` (the
// tab check always sits leftmost in this codebase's guards).
func tabEquals(cond ast.Expr) (int, bool) {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok {
		return 0, false
	}
	if bin.Op == token.LAND {
		return tabEquals(bin.X)
	}
	if bin.Op != token.EQL {
		return 0, false
	}
	sel, ok := bin.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "tab" {
		return 0, false
	}
	lit, ok := bin.Y.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return 0, false
	}
	n, err := strconv.Atoi(lit.Value)
	if err != nil {
		return 0, false
	}
	return n, true
}

// isDirectClientCall reports whether call is mutateThenRefresh(...) or m.action(...) — the two
// helpers that reach the hub without an intervening confirm.
func isDirectClientCall(call *ast.CallExpr) bool {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name == "mutateThenRefresh"
	case *ast.SelectorExpr:
		return fn.Sel.Name == "action"
	}
	return false
}

// hasDirectClientCall walks body's statements for a direct client call, never descending into a
// func literal — that shape is always a deferred confirm/apply, never the bare keystroke itself.
func hasDirectClientCall(body *ast.BlockStmt) bool {
	found := false
	var walkExpr func(ast.Expr)
	walkExpr = func(e ast.Expr) {
		if e == nil || found {
			return
		}
		switch x := e.(type) {
		case *ast.CallExpr:
			if isDirectClientCall(x) {
				found = true
				return
			}
			for _, a := range x.Args {
				walkExpr(a)
			}
		case *ast.BinaryExpr:
			walkExpr(x.X)
			walkExpr(x.Y)
		case *ast.UnaryExpr:
			walkExpr(x.X)
		case *ast.ParenExpr:
			walkExpr(x.X)
		}
		// *ast.FuncLit and anything else: deliberately not descended into.
	}
	var walkStmt func(ast.Stmt)
	walkStmt = func(s ast.Stmt) {
		if s == nil || found {
			return
		}
		switch x := s.(type) {
		case *ast.ReturnStmt:
			for _, r := range x.Results {
				walkExpr(r)
			}
		case *ast.ExprStmt:
			walkExpr(x.X)
		case *ast.AssignStmt:
			for _, r := range x.Rhs {
				walkExpr(r)
			}
		case *ast.IfStmt:
			walkStmt(x.Body)
			if x.Else != nil {
				walkStmt(x.Else)
			}
		case *ast.BlockStmt:
			for _, s2 := range x.List {
				walkStmt(s2)
			}
		}
	}
	walkStmt(body)
	return found
}

// TestKeysThatCommitDirectlyAreMarkedSoInTheKeymap is the invariant sd-6d0ff2 asks for: a handler
// that reaches the client on the bare keystroke (mutateThenRefresh, m.action) must be commits:true,
// derived from onkey.go itself so the classification cannot drift the way the first pass did.
// Which harmless bindings also move behind the prefix to reclaim footer space is a judgement this
// cannot express — that part is curated, not derived (and is not this test's job).
func TestKeysThatCommitDirectlyAreMarkedSoInTheKeymap(t *testing.T) {
	pairs := directCommitPairs(t)
	if len(pairs) < 2 {
		t.Fatalf("only found %d direct-commit keys — the AST derivation may not be reading onkey.go", len(pairs))
	}
	for key, tabs := range pairs {
		for _, tab := range tabs {
			scope := tabScope(tab)
			found := false
			for _, b := range keymap {
				if b.scope != scope {
					continue
				}
				for _, k := range menuKeys(b.keys) {
					if k != key {
						continue
					}
					found = true
					if !b.commits {
						t.Errorf("%q on tab %d (scope %d) reaches the client directly on the bare keystroke but its keymap row is not commits:true", key, tab, scope)
					}
				}
			}
			if !found {
				t.Errorf("%q on tab %d (scope %d) reaches the client directly but has no matching keymap row", key, tab, scope)
			}
		}
	}
}

// TestTabsInChainFollowsElseIf is the regression the else-if fix closes: onkey.go's multi-tab
// cases are almost always written that way (keyFilter, keyNew, keyTell among them), and a check
// that stopped at the first branch would never see a direct client call added to a later one.
func TestTabsInChainFollowsElseIf(t *testing.T) {
	src := `package p
func f() {
	if m.tab == 0 {
		m.flash = "no-op"
	} else if m.tab == 2 {
		return mutateThenRefresh(nil, nil)
	} else if m.tab == 4 {
		m.flash = "also no-op"
	}
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	var chain *ast.IfStmt
	ast.Inspect(file, func(n ast.Node) bool {
		if s, ok := n.(*ast.IfStmt); ok && chain == nil {
			chain = s
		}
		return chain == nil
	})
	if chain == nil {
		t.Fatal("did not find the if-chain in the fixture")
	}
	if got := tabsInChain(chain); len(got) != 1 || got[0] != 2 {
		t.Errorf("tabsInChain should find the direct call on tab 2 inside the else-if branch, got %v", got)
	}
}
