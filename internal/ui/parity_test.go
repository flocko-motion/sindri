package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// oneSided names the client methods deliberately reachable from ONE front-end, with the reason each
// is not a parity gap (ARCHITECTURE.md:42). An entry here is an argument in writing — the exception
// has to be defended rather than tolerated.
var oneSided = map[string]string{
	"Close":         "lifecycle of the client itself, not a user-facing behaviour",
	"Directive":     "the agent's own loop inside a pod, reached by neither front-end's user",
	"Exec":          "the agent socket's verb dispatch, not a front-end action",
	"Commands":      "the agent's contextual verb list, for the pod's own help",
	"ChatHeartbeat": "the TUI holds the meeting room open; the CLI's chat is one-shot",
	"ChatWatch":     "a live stream only a long-running screen consumes",
	"Watch":         "as ChatWatch: the board stream is the TUI's, the CLI reads once",

	// Same behaviour, different route. The rule is about what a user can DO — but each is written
	// out, because "it's fine" is what an unexamined gap says too.
	"Instance": "`agent info` shows the pod through this; the TUI's detail pane shows it through " +
		"PodInfo. One behaviour, two methods — worth collapsing, not a parity hole.",
	"PodInfo": "the TUI half of the Instance pair above",
	"Repos": "the TUI reads the repo list off the board (BoardState.Projects) rather than asking " +
		"for it; the Repos tab is the same behaviour",
	"RepoInit": "the TUI registers the repo it is started in implicitly, on first use, so there is " +
		"nothing for a user to invoke",
	"ReconcileTasks": "the TUI sweeps at startup; the CLI's `task list` reconciles hub-side on every " +
		"call, so the repair happens either way and neither user asks for it",
	"ScrapPR": "the TUI scraps a task's PR alongside the task; the CLI reaches the same thing with " +
		"`task delete --prs`, which the hub applies server-side",
}

// TestEveryClientMethodIsReachableFromBothFrontEnds enforces ARCHITECTURE.md:42. A method only one
// front-end calls is a behaviour the other cannot perform. Tracked by hand it drifted (-> td-10c59e)
// and drifted again, so it is checked.
func TestEveryClientMethodIsReachableFromBothFrontEnds(t *testing.T) {
	methods := clientMethods(t, "../client")
	if len(methods) < 40 {
		t.Fatalf("only %d client methods found — the parser is not reading internal/client", len(methods))
	}
	cli, tui := selectors(t, "cli"), selectors(t, "tui")
	// Prove both sides were actually read: an empty set would pass every method vacuously.
	for name, got := range map[string]map[string]bool{"cli": cli, "tui": tui} {
		if len(got) < 50 {
			t.Fatalf("only %d selectors found in internal/ui/%s — the parser is not reading it", len(got), name)
		}
	}

	var gaps []string
	for _, m := range methods {
		if _, ok := oneSided[m]; ok {
			continue
		}
		switch {
		case !cli[m] && !tui[m]:
			gaps = append(gaps, m+": reachable from neither front-end (dead, or reached some other way)")
		case !tui[m]:
			gaps = append(gaps, m+": CLI only — no key or action reaches it in the TUI")
		case !cli[m]:
			gaps = append(gaps, m+": TUI only — no command reaches it in the CLI")
		}
	}
	sort.Strings(gaps)
	for _, g := range gaps {
		t.Errorf("front-end parity: %s", g)
	}
}

// clientMethods lists the exported methods on *HTTP — the front-ends' whole door to the core.
func clientMethods(t *testing.T, dir string) []string {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	var out []string
	for _, p := range pkgs {
		for name, f := range p.Files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			for _, d := range f.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || !fn.Name.IsExported() {
					continue
				}
				if star, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
					if id, ok := star.X.(*ast.Ident); ok && id.Name == "HTTP" {
						out = append(out, fn.Name.Name)
					}
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// selectors collects every x.Name referenced under a front-end — names, not resolved types, since
// the CLI goes through its own backend interface and the TUI through a held handle. Over-generous
// by construction, so a reported gap is always real; the counts above guard the other direction.
func selectors(t *testing.T, front string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	root := filepath.Join("..", "ui", front)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			return perr
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				out[sel.Sel.Name] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}
