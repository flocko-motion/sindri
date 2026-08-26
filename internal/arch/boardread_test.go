// package: arch / boardread
// job:     fail the build if a board read queries anything — every call reachable from
// hub.State must land on the store, the watchdog's standing observations, or a
// declared pure helper, and never on the container runtime, a session transcript
// or the filesystem.
// type:    test (architecture invariant)
// limits:  the board read only; what the observer itself may poll is its own business
// (-> hub/watchdog.go), and the probe sites on other paths are theirs.
package arch

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

// queryingImports are the packages a board read must never take an action through: the container
// runtime, an adapter that shells out or parses a transcript, the filesystem, a subprocess. Reaching
// one costs a spawn or a read per call — per agent, per render, times the connected clients, which is
// the failure this guard exists for. A pure accessor in one of them is still reachable, but only by
// name and only with a reason (-> boardCalls); the package as a whole is never waved through.
var queryingImports = map[string]string{
	"github.com/flo-at/sindri/internal/container":      "a container operation — a process spawn per call",
	"github.com/flo-at/sindri/internal/adapter/agent":  "the coding-agent backend — reads session transcripts off disk",
	"github.com/flo-at/sindri/internal/adapter/tmux":   "tmux — only reachable by exec'ing into a pod",
	"github.com/flo-at/sindri/internal/adapter/git":    "git — a subprocess per call",
	"github.com/flo-at/sindri/internal/adapter/github": "the GitHub CLI — a network round trip",
	"github.com/flo-at/sindri/internal/config":         "a repo's config file — a filesystem read",
	"os":      "the filesystem",
	"os/exec": "a subprocess",
}

// boardHandles are the receivers a board read may call anything on, each with why its whole surface
// is free of queries. Keyed by the expression text the call is made through, so a local taken from
// one of them (ps := h.store.For(...)) counts as the same handle.
var boardHandles = map[string]string{
	"h.store":     "the central store IS the read model — every board fact is a row",
	"h.projects":  "the repo registry, a store table",
	"h.chat":      "the chat service, a projection of its own tables",
	"h.comments":  "the comment service, likewise",
	"h.events":    "the in-memory pub/sub the board notifies through",
	"h.watch":     "the watchdog's standing observations — what a board read reports",
	"w.mu":        "a mutex, guarding one of those observations",
	"h.mu":        "likewise",
	"h.startedAt": "the hub's start time, a value it has held since New",
	// Pure packages: values and formatting, no I/O of any kind.
	"api":      "wire types and the status vocabulary",
	"commands": "resolves the board's own tabs from the board",
	"store":    "row types and their helpers",
	"agent":    "the agent package's pure helpers (see boardCalls for its service)",
	"workflow": "the workflow package's pure rules (see boardCalls for its engine)",
	"fmt":      "formatting",
	"strings":  "text",
	"sort":     "ordering",
	"time":     "clock arithmetic",
	"filepath": "path arithmetic, no stat",
	"log":      "the hub's own log",
	"errors":   "error values",
}

// boardCalls are the individual methods a board read may reach on a handle that is NOT wholesale
// safe. The agent service and the workflow engine each span both kinds — store reads and live probes
// — so every method a render uses off them is weighed one at a time, here, in writing.
//
// This is the list that would have caught the regression. State grew a CurrentModel call, which
// opened with an untimed liveness probe: two container operations per agent, on every board read, for
// every connected client. The rule it broke was already written in watchdog.go's header, and prose in
// a header cannot fail a build.
var boardCalls = map[string]string{
	"h.agents.AgentStatus":     "folds liveness + phase into a word; in-memory, no probe",
	"h.agents.Unreachable":     "counts pushes the pane never showed — a map the injector keeps, no probe",
	"h.wf.FleetRuns":           "ranks the runs table — a store read",
	"container.Name":           "the backend's display name, a string it holds — not an operation on it",
	"container.AgentContainer": "builds a pod name out of a path — string arithmetic, not a pod",
}

// hubInternal maps each handle onto the hub type behind it, so a call through one is followed into
// the hub's own code rather than stopping. Written down rather than derived: a new internal handle
// stops the walk and asks to be declared, which is the same bargain as the lists above.
var hubInternal = map[string]string{
	"h":       "Hub",
	"w":       "watchdog",
	"h.watch": "watchdog",
}

// TestABoardReadQueriesNothing walks every function hub.State reaches inside package hub and checks
// what each of them calls out to. A call into a querying package fails outright; anything else must
// be on one of the two lists above, so a new reach is a deliberate, written-down decision rather than
// a line that looked harmless in review.
func TestABoardReadQueriesNothing(t *testing.T) {
	pkg := parseHubPackage(t, filepath.Join(moduleRoot(t), "internal", "hub"))
	const board = "Hub.State"
	if pkg.decls[board] == nil {
		t.Fatal("hub.State not found — the guard is reading the wrong package")
	}

	var problems []string
	leaves := 0
	walked := map[string]bool{}
	queue := []string{board}
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		if walked[key] {
			continue
		}
		walked[key] = true
		for _, decl := range pkg.decls[key] {
			for _, c := range callsIn(decl.fn) {
				if c.root == "" { // a bare name: the package's own func, or a builtin/conversion
					if pkg.decls["."+c.name] != nil {
						queue = append(queue, "."+c.name)
					}
					continue
				}
				if recv, internal := hubInternal[c.root]; internal && pkg.decls[recv+"."+c.name] != nil {
					queue = append(queue, recv+"."+c.name)
					continue
				}
				leaves++
				reach := c.root + "." + c.name
				if _, weighed := boardCalls[reach]; weighed {
					continue
				}
				if why, forbidden := queryingImports[decl.imports[c.root]]; forbidden {
					problems = append(problems, key+" reaches "+reach+" — "+why)
					continue
				}
				if declaredHandle(c.root) {
					continue
				}
				problems = append(problems, key+" reaches "+reach+", which no list says is free of queries")
			}
		}
	}

	// Anti-vacuity: a board read is assembled from dozens of store calls across a dozen helpers, so a
	// walk that found almost nothing is a broken walk rather than a clean one.
	if len(walked) < 8 || leaves < 20 {
		t.Fatalf("the walk reached %d functions and %d outward calls — it is not reading hub.State",
			len(walked), leaves)
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Errorf("a board read must touch only the store and the watchdog: %s.\n"+
			"Sample it on the watchdog's sweep and report the last observation, or — if it truly costs "+
			"nothing per render — add it to boardHandles/boardCalls with the reason", p)
	}
}

// declaredHandle reports whether a root is a declared handle, or data reached through one — the
// watchdog's own observation carries values, and reading a field off it is still reading the observer.
func declaredHandle(root string) bool {
	for h := range boardHandles {
		if root == h || strings.HasPrefix(root, h+".") {
			return true
		}
	}
	return false
}

// hubPackage is package hub's own declarations, keyed "<receiver type>.<name>" (empty receiver for a
// plain func), each with the imports its file declared so a package qualifier in a call can be
// resolved to the path it means. Keyed by receiver because two types here declare a Container method,
// and a name-only key walked both.
type hubPackage struct {
	decls map[string][]hubDecl
}

type hubDecl struct {
	fn      *ast.FuncDecl
	imports map[string]string // local package name -> import path
}

// parseHubPackage reads the non-test files of package hub. Only that directory: the walk follows the
// hub's own helpers and treats everything else as a leaf to be declared, which is what keeps the
// judgement in the lists rather than in a name-matched guess about another package's methods.
func parseHubPackage(t *testing.T, dir string) hubPackage {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	pkg := hubPackage{decls: map[string][]hubDecl{}}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, e.Name()), nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", e.Name(), perr)
		}
		imports := map[string]string{}
		for _, im := range f.Imports {
			path := strings.Trim(im.Path.Value, `"`)
			local := path[strings.LastIndex(path, "/")+1:]
			if im.Name != nil {
				local = im.Name.Name
			}
			imports[local] = path
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			key := receiverType(fn) + "." + fn.Name.Name
			pkg.decls[key] = append(pkg.decls[key], hubDecl{fn: fn, imports: imports})
		}
	}
	return pkg
}

// receiverType names the type a method hangs off, "" for a plain func.
func receiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	t := fn.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// outward is one call a function makes: the expression it is called through (root, "" for a bare
// name) and the name called.
type outward struct{ root, name string }

// callsIn lists every call a function body makes, with locals resolved back to the handle they came
// from — ps := h.store.For(tag) makes ps.GetState a call on h.store, which is what it is.
func callsIn(fn *ast.FuncDecl) []outward {
	locals := map[string]string{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Rhs) != 1 {
			return true
		}
		// One right-hand side, however many results: they all came through the same handle, which is
		// what an observation destructured into up/clients/runtime is.
		src := rootOf(as.Rhs[0], locals)
		if src == "" {
			return true
		}
		for _, l := range as.Lhs {
			if id, ok := l.(*ast.Ident); ok {
				locals[id.Name] = src
			}
		}
		return true
	})

	var out []outward
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch f := call.Fun.(type) {
		case *ast.Ident:
			out = append(out, outward{name: f.Name})
		case *ast.SelectorExpr:
			if root := rootOf(f.X, locals); root != "" {
				out = append(out, outward{root: root, name: f.Sel.Name})
			}
		}
		return true
	})
	return out
}

// rootOf is the handle an expression is reached through, as text: h.store for h.store.For(tag) and
// for any local assigned from it, "" for anything the guard cannot name.
func rootOf(e ast.Expr, locals map[string]string) string {
	switch x := e.(type) {
	case *ast.Ident:
		if src, ok := locals[x.Name]; ok {
			return src
		}
		return x.Name
	case *ast.SelectorExpr:
		if base := rootOf(x.X, locals); base != "" {
			return base + "." + x.Sel.Name
		}
	case *ast.CallExpr:
		// The value a call returns is reached through the call's own receiver: ps := h.store.For(tag)
		// makes ps a handle on h.store, not on h.store.For.
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
			return rootOf(sel.X, locals)
		}
	case *ast.IndexExpr:
		return rootOf(x.X, locals)
	case *ast.StarExpr:
		return rootOf(x.X, locals)
	case *ast.UnaryExpr:
		return rootOf(x.X, locals)
	case *ast.ParenExpr:
		return rootOf(x.X, locals)
	}
	return ""
}
