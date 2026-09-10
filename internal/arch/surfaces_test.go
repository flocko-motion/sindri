// package: arch / surfaces_test
// type:    test (architecture invariant)
// job:     fail the build when the hub's two command surfaces reach into each other, and when a
// verb or a route decides an operation both of them drive instead of asking the one core
// that owns it (-> hub/api).
// limits:  the reach and the call sites. Whether an operation is RIGHT is its subject's own tests'.
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// agentSurface and frontSurface are the two directories the split exists to keep apart: what an
// agent types, and what the host CLI and the TUI drive. They answer different callers in different
// shapes — argv and prose against JSON — so a file in one reaching the other is a surface starting
// to grow the other's contract.
const (
	agentSurface = "internal/hub/api/agents"
	frontSurface = "internal/hub/api/frontend"
)

// TestTheTwoSurfacesDoNotReachEachOther walks both trees for an import of the other.
func TestTheTwoSurfacesDoNotReachEachOther(t *testing.T) {
	root := moduleRoot(t)
	pairs := []struct{ from, to string }{{agentSurface, frontSurface}, {frontSurface, agentSurface}}
	seen := 0
	for _, p := range pairs {
		err := filepath.WalkDir(filepath.Join(root, p.from), func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return walkErr
			}
			f, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if perr != nil {
				return perr
			}
			seen++
			rel, _ := filepath.Rel(root, path)
			for _, imp := range f.Imports {
				if strings.Contains(strings.Trim(imp.Path.Value, `"`), p.to) {
					t.Errorf("%s imports %s — the agent surface and the front-end answer different "+
						"callers in different shapes, so whatever they share belongs beside them "+
						"(-> hub/api/serve) rather than in one of them", filepath.ToSlash(rel), p.to)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", p.from, err)
		}
	}
	if seen < 4 {
		t.Fatalf("only %d files scanned across the two surfaces — the scan is not reading them", seen)
	}
}

// forkedOperations are the operations both surfaces drive, each named by the exported core that
// owns it. A verb adapter and a route adapter may parse, render and gate; the decision itself is
// the core's, once. Written twice, `approve` drifted — the human path silently lacked the guards
// and the log entry the agent's had, and nothing kept the pair in step.
var forkedOperations = map[string]string{
	"approve": "flow/pr's approve, told who is ruling",
	"reject":  "flow/pr's reject, told whose voice the feedback speaks in",
	"resume":  "flow/agent's Resume, whoever cleared the escalation",
}

// TestNoOperationIsImplementedTwice reads the pair of entry points for each forked operation and
// requires both to reach the same unexported core. Reading the call graph properly is gopls' job,
// so this checks the cheap shape that actually regressed: the two entry points must not each carry
// the store writes that settle the operation.
func TestNoOperationIsImplementedTwice(t *testing.T) {
	root := moduleRoot(t)
	// The verb adapter and the route adapter for each operation, as (file, funcs) — the pairs that
	// have to agree. A new forked operation is added here, or it is not covered.
	pairs := map[string][2]string{
		"approve": {"internal/hub/flow/pr/verdict_act.go", "CmdApprove|ApprovePR"},
		"reject":  {"internal/hub/flow/pr/verdict_act.go", "CmdReject|RejectPR"},
		"resume":  {"internal/hub/flow/agent/escalate.go", "CmdResume|ResumeByUser"},
	}
	var missing []string
	for op := range forkedOperations {
		if _, ok := pairs[op]; !ok {
			missing = append(missing, op)
		}
	}
	sort.Strings(missing)
	for _, op := range missing {
		t.Errorf("operation %q is declared forked but names no pair of entry points to check", op)
	}

	for op, pair := range pairs {
		path := filepath.Join(root, pair[0])
		for _, fn := range strings.Split(pair[1], "|") {
			body, ok := funcBody(t, path, fn)
			if !ok {
				t.Errorf("%s: %s is gone — the pair this guard watches for %q no longer exists, so "+
					"either update the pair or drop the entry", pair[0], fn, op)
				continue
			}
			// PutPR is the write that settles a verdict. Exactly one implementation may carry it, and
			// that is the shared core — never an adapter over it.
			if strings.Contains(body, "PutPR(") {
				t.Errorf("%s: %s writes the PR itself rather than asking %s — an operation implemented "+
					"in its adapter is one that will drift from the other surface's copy",
					pair[0], fn, forkedOperations[op])
			}
		}
	}
}

// funcBody is the source text of one function or method in path, ok=false when it has none.
func funcBody(t *testing.T, path, name string) (string, bool) {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != name || fn.Body == nil {
			continue
		}
		return string(src[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset]), true
	}
	return "", false
}
