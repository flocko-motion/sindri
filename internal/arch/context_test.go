// package: arch / context
// type:    test (architecture invariant)
// job:     fail the build if a fresh context root appears below an entrypoint — every
// context.Background()/TODO() outside the files listed here must take the caller's
// context and narrow it (-> ARCHITECTURE.md, "Context is handed through, never invented").
// limits:  the roots only. Whether a bound is the right length, and which calls need one at
// all, belongs to the code that makes them; tests may root freely, since a test IS an
// entrypoint (t.Context() is the better one, and most use it).
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// declaredRoots are the files allowed to start a context, each with WHY it is an entrypoint. A root
// anywhere else cuts two wires at once: cancellation stops reaching the work, so an operation its
// caller abandoned runs on, and the fresh root carries no deadline to inherit, so a wedged dependency
// blocks its caller for ever. The second has cost an outage here — a liveness probe on Background
// held every board read open until podman answered.
//
// The list is the point. Prose in ARCHITECTURE.md tells a reviewer what to look for; only a test
// stops the next Background from looking harmless in a diff.
var declaredRoots = map[string]string{
	// The hub process: one root, handed to hub.New as the hub's lifetime, inherited by every loop it
	// starts and every fleet-side push whose port carries no context of its own (-> hub.Hub.lifetime).
	"cmd/sindri-hub/main.go": "the hub entrypoint — the root the whole hub tree hangs off",
	// The TUI's interactive sessions: a front-end's own lifetime, ended when it exits. The CLI chat
	// session takes cmd.Context() instead (cobra roots it in Execute, cmd/sindri/main.go) — it is a
	// caller with a context already in scope, not a fresh entrypoint.
	"internal/ui/tui/startup.go":  "the TUI's Run — a front-end entrypoint",
	"internal/ui/tui/switcher.go": "the repo switcher's own probe lifetime, cancelled on selection",
	// A poller and a version check, each its own background work with nobody above it to inherit from.
	"internal/ui/attach/herdr.go": "the attach view's refresh poll",
	"internal/update/update.go":   "the updater's own fetch",
	// brokkr is its own binary: this is a linter run, started by its command.
	"internal/brokkr/lint/jslint.go": "brokkr's js linter — its own tool run",
	// Adapters bounding their own call where the PORT METHOD carries no context to inherit:
	// container.Runtime.Healthy and task.Source's reads. Each is bounded, so none can hang a caller;
	// widening those two ports to carry one is a change to the ports, not to this rule.
	"internal/adapter/container/pod/pod.go":                       "Healthy's 3s reachability probe — the port method takes no ctx",
	"internal/adapter/container/applecontainer/applecontainer.go": "the same probe on the Apple backend",
	"internal/adapter/tasks/github/github.go":                     "task.Source's reads, each bounded by issueTimeout — the port takes no ctx",
}

// TestNoContextRootBelowAnEntrypoint walks the module for context.Background() and context.TODO(),
// and requires each file that starts one to be on the declared list.
func TestNoContextRootBelowAnEntrypoint(t *testing.T) {
	root := moduleRoot(t)
	found := map[string]bool{}
	var undeclared []string
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
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			return nil // not parseable is brokkr lint's business, not this guard's
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if !isContextRoot(n) {
				return true
			}
			seen++
			if _, declared := declaredRoots[rel]; declared {
				found[rel] = true
				return true
			}
			undeclared = append(undeclared, rel)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	// Anti-vacuity: the declared roots are real call sites, so a scan finding none is a broken scan
	// rather than a clean tree — which is how a guard becomes decoration.
	if seen < len(declaredRoots) {
		t.Fatalf("only %d context roots found across the module — the scan is not reading it", seen)
	}
	for _, u := range dedupe(undeclared) {
		t.Errorf("%s starts a context of its own. Take the caller's and narrow it — "+
			"context.WithTimeout(ctx, probeTimeout) around a probe — or, if this really is where the "+
			"work starts, add the file to declaredRoots with the reason", u)
	}
	// A declared root that no longer roots anything is a reason nobody has to justify any more: the
	// list only holds its meaning while every line on it is still load-bearing.
	for rel, why := range declaredRoots {
		if !found[rel] {
			t.Errorf("declaredRoots lists %s (%q) but it starts no context any more — drop the entry", rel, why)
		}
	}
}

// isContextRoot reports whether n is a context.Background() or context.TODO() call. Both, because
// TODO is Background with an apology, and an apology is not a lineage.
func isContextRoot(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "context" {
		return false
	}
	return sel.Sel.Name == "Background" || sel.Sel.Name == "TODO"
}

// dedupe collapses repeats so a file with four roots is reported once, not four times.
func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
