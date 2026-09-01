// package: arch / harness
// type:    test (architecture invariant)
// job:     hold the seam between the box and the work, in the ways it has actually been broken: the
// orchestrator may not reach the box's adapters nor match a word off its pane, the observation may
// carry no judgement, the harness may name nothing the orchestrator owns, and the delivery path may
// not ask the ruleset for permission.
// limits:  these. Whether a rule is RIGHT is the surface's own tests'.
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	// Blank-imported so this test's cache key includes their transitive source: `go list -deps` below
	// is a subprocess Go's cache cannot see into, so without these edges a new import under one of
	// them would leave a stale PASS here.
	_ "github.com/flo-at/sindri/internal/hub/situation"
	_ "github.com/flo-at/sindri/internal/hub/workflow"
)

// boxAdapters are the packages that ARE an agent's box: the session, and the coding tool inside it.
// The orchestrator reaches both through the harness, so its import graph must not contain either at
// all — the mechanical half of "workflow names no tmux operation".
//
// internal/container is NOT here, and deliberately: the run queue builds and executes in disposable
// pods that belong to no agent, which is a container operation without being a reach into anyone's
// box. That exception is held to the file that owns it instead (-> runQueuePods).
var boxAdapters = map[string]string{
	"github.com/flo-at/sindri/internal/adapter/tmux":  "tmux, the session itself",
	"github.com/flo-at/sindri/internal/adapter/agent": "the coding tool running in it",
}

// runQueuePods are the orchestrator files allowed to name the container runtime, with why. A pod an
// AGENT lives in is never one of them: that is the harness's, and a file added here claiming
// otherwise is the thing this list exists to make somebody argue for.
var runQueuePods = map[string]string{
	"internal/hub/workflow/execrun.go": "the run queue's own throwaway pods, which belong to no agent",
}

// TestOnlyTheRunQueueNamesTheRuntime is the file-level half of the rule above: the import graph
// cannot tell execrun.go's disposable pods from a reach into an agent's session, so the exception is
// pinned to the file rather than to the package.
func TestOnlyTheRunQueueNamesTheRuntime(t *testing.T) {
	const runtime = `"github.com/flo-at/sindri/internal/container"`
	seen := 0
	for _, dir := range []string{"internal/hub/workflow", "internal/hub/situation"} {
		entries, err := os.ReadDir(filepath.Join(moduleRoot(t), dir))
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			seen++
			rel := dir + "/" + e.Name()
			body, rerr := os.ReadFile(filepath.Join(moduleRoot(t), rel))
			if rerr != nil {
				t.Fatalf("read %s: %v", rel, rerr)
			}
			if !strings.Contains(string(body), runtime) {
				continue
			}
			if _, declared := runQueuePods[rel]; !declared {
				t.Errorf("%s names the container runtime. An agent's pod is the harness's to touch; if "+
					"this is a pod no agent lives in, add the file to runQueuePods with the reason", rel)
			}
		}
	}
	if seen < 20 {
		t.Fatalf("only %d orchestrator files scanned — the guard is reading the wrong directories", seen)
	}
}

// orchestrators are the packages that decide what should happen, each of which must stay clear of
// the box. situation is here as well as workflow: it is where the rules moved TO, so it inherits the
// same prohibition, and a rule that reached for a pane would land there first.
var orchestrators = []string{
	"github.com/flo-at/sindri/internal/hub/workflow",
	"github.com/flo-at/sindri/internal/hub/situation",
}

// TestTheOrchestratorCannotReachTheBox walks the REAL import graph, so an indirect import fails too —
// one the orchestrator picks up through a package it already uses, which a name-based check misses.
// The five violations this epic removed accumulated because nothing checked; two of them were true
// again and held by nothing until this test.
func TestTheOrchestratorCannotReachTheBox(t *testing.T) {
	for _, pkg := range orchestrators {
		out, err := exec.Command("go", "list", "-deps", pkg).Output()
		if err != nil {
			t.Fatalf("go list -deps %s: %v", pkg, err)
		}
		deps := strings.Split(strings.TrimSpace(string(out)), "\n")
		// Prove the graph was read before trusting a clean sweep of it: a package that moved, or a
		// list that came back empty, would otherwise report the invariant holding vacuously.
		if len(deps) < 10 {
			t.Fatalf("go list -deps %s returned %d packages — the guard is not reading the graph", pkg, len(deps))
		}
		for _, dep := range deps {
			if why, forbidden := boxAdapters[dep]; forbidden {
				t.Errorf("%s depends on %s — %s. The orchestrator reaches the box only through "+
					"workflow.Harness; if this arrived indirectly, the package that pulled it in is the "+
					"one to fix", pkg, dep, why)
			}
		}
	}
}

// TestDeliveryDoesNotAskTheRuleset walks out from hub.Deliver and fails on any reach into the
// workflow engine. Asking it whether a message is worth sending is what made the ruleset and the
// delivery path call each other through the composition root: the sender decides, and delivery
// carries out what it is given (-> hub/deliver.go).
func TestDeliveryDoesNotAskTheRuleset(t *testing.T) {
	pkg := parseHubPackage(t, filepath.Join(moduleRoot(t), "internal", "hub"))
	const entry = "Hub.Deliver"
	if pkg.decls[entry] == nil {
		t.Fatal("hub.Deliver not found — the guard is reading the wrong package")
	}
	walked, reached := map[string]bool{}, 0
	queue := []string{entry}
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		if walked[key] {
			continue
		}
		walked[key] = true
		for _, decl := range pkg.decls[key] {
			for _, c := range callsIn(decl.fn) {
				reached++
				if c.root == "h.wf" {
					t.Errorf("%s reaches h.wf.%s — delivery must not ask the ruleset whether a message is "+
						"worth sending. That judgement belongs where the message is composed", key, c.name)
					continue
				}
				if c.root == "" && pkg.decls["."+c.name] != nil {
					queue = append(queue, "."+c.name)
					continue
				}
				if recv, internal := hubInternal[c.root]; internal && pkg.decls[recv+"."+c.name] != nil {
					queue = append(queue, recv+"."+c.name)
				}
			}
		}
	}
	// Anti-vacuity: Deliver writes mail, notifies the board and injects, across more than one helper.
	if reached < 8 {
		t.Fatalf("the walk reached %d calls from hub.Deliver — it is not reading the delivery path", reached)
	}
}

// TestNoPaneWordIsMatchedInTheHub is the guard whose absence let a pane word survive a green gate:
// workflow/stall.go still matched "api-error" off a raw runtime parameter after the other two strings
// had gone, and stallwatch and credwatch each matched one of their own. The words come OUT of
// observe.go's const block rather than being restated here, so a sixth word added there is guarded
// the day it appears.
//
// The TYPE is the real defence now — observe.State is an integer, so a literal will not compile
// against it. This is the backstop for the word that still travels: AgentView.Runtime crosses the
// wire as a string, and a hub file matching that would be the same mistake wearing a different hat.
//
// It flags a word compared against, or assigned to, something NAMED for the runtime — which is what
// makes it precise. "idle" and "working" are also the workflow's own PHASE vocabulary, and a guard on
// the bare literals would flag fifty honest `st.Phase == "working"` sites and be switched off.
//
// The whole hub, not just the orchestrator: three of the five instances were on the observer's side
// of the seam, where the words are just as much evidence to be asked about rather than matched.
func TestNoPaneWordIsMatchedInTheHub(t *testing.T) {
	words := paneWords(t)
	if len(words) < 5 {
		t.Fatalf("only %d runtime words read out of observe.go — the guard is not reading the vocabulary", len(words))
	}
	files, hits := 0, 0
	for _, dir := range []string{"internal/hub", "internal/hub/workflow", "internal/hub/situation"} {
		entries, err := os.ReadDir(filepath.Join(moduleRoot(t), dir))
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			files++
			rel := dir + "/" + e.Name()
			f, perr := parser.ParseFile(token.NewFileSet(), filepath.Join(moduleRoot(t), rel), nil, 0)
			if perr != nil {
				t.Fatalf("parse %s: %v", rel, perr)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				for _, word := range paneWordUses(n, words) {
					hits++
					t.Errorf("%s reads the pane word %q off a runtime directly. The session's own account "+
						"crosses as evidence to be INTERPRETED: ask the observation (AtPrompt, Working, "+
						"AwaitingHuman, SignedOut, TurnCutOff), never the string", rel, word)
				}
				return true
			})
		}
	}
	if files < 40 {
		t.Fatalf("only %d hub files scanned — the guard is reading the wrong directories", files)
	}
	_ = hits
}

// paneWords reads the vocabulary out of the one table that holds it (-> observe/state.go's words),
// so this guard and the code it guards cannot hold different ideas of what the words are.
func paneWords(t *testing.T) map[string]bool {
	t.Helper()
	path := filepath.Join(moduleRoot(t), "internal", "hub", "observe", "state.go")
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	words := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range lit.Elts {
			kv, isKV := elt.(*ast.KeyValueExpr)
			if !isKV {
				continue
			}
			if v, isLit := kv.Value.(*ast.BasicLit); isLit && v.Kind == token.STRING {
				words[strings.Trim(v.Value, `"`)] = true
			}
		}
		return true
	})
	return words
}

// paneWordUses reports the pane words this node reads off something named for the runtime — a
// comparison against it, or an assignment into it.
func paneWordUses(n ast.Node, words map[string]bool) []string {
	var found []string
	add := func(a, b ast.Expr) {
		if namesTheRuntime(a) {
			if lit, ok := b.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if w := strings.Trim(lit.Value, `"`); words[w] {
					found = append(found, w)
				}
			}
		}
	}
	switch e := n.(type) {
	case *ast.BinaryExpr:
		if e.Op == token.EQL || e.Op == token.NEQ {
			add(e.X, e.Y)
			add(e.Y, e.X)
		}
	case *ast.AssignStmt:
		for i, lhs := range e.Lhs {
			if i < len(e.Rhs) {
				add(lhs, e.Rhs[i])
			}
		}
	case *ast.SwitchStmt:
		// `switch runtime { case "api-error": }` reaches the same conclusion by another route. The TAG
		// is checked, not the cases alone: `switch st.Phase { case "working": }` is the workflow's own
		// vocabulary and must not be flagged.
		if !namesTheRuntime(e.Tag) {
			return nil
		}
		for _, stmt := range e.Body.List {
			clause, ok := stmt.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, v := range clause.List {
				if lit, isLit := v.(*ast.BasicLit); isLit && lit.Kind == token.STRING {
					if w := strings.Trim(lit.Value, `"`); words[w] {
						found = append(found, w)
					}
				}
			}
		}
	}
	return found
}

// namesTheRuntime reports whether an expression is called `runtime` — a bare local, a parameter, or
// a field like o.Runtime. The name is the signal: nothing else in these packages is called that.
func namesTheRuntime(e ast.Expr) bool {
	switch x := e.(type) {
	case nil:
		return false
	case *ast.Ident:
		return strings.EqualFold(x.Name, "runtime")
	case *ast.SelectorExpr:
		return strings.EqualFold(x.Sel.Name, "runtime")
	}
	return false
}

// judgements are conclusions about an agent's WORK. Each is true only in the light of what the agent
// holds, so none can be read off a box — and a field or method here carrying one would be the harness
// deciding something it cannot see.
var judgements = []string{"stalled", "blocked", "full", "needs", "idle"}

// TestTheObservationCarriesNoJudgement walks the observation's own package for an exported name that
// states a conclusion. "Idle" is the one to watch: "at an empty prompt" is an observation and "has
// nothing to do" is a conclusion, and one identifier carried both until this split.
func TestTheObservationCarriesNoJudgement(t *testing.T) {
	names, files := exportedNamesIn(t, filepath.Join(moduleRoot(t), "internal", "hub", "observe"))
	if files < 1 {
		t.Fatal("no source found in internal/hub/observe — the scan is reading the wrong directory")
	}
	if len(names) < 8 {
		t.Fatalf("only %d exported names found — the scan is not reading the observation", len(names))
	}
	for _, n := range names {
		for _, word := range judgements {
			if strings.Contains(strings.ToLower(n), word) {
				t.Errorf("the observation exports %q, which states a %q — that is true only against the "+
					"work the agent holds, so it belongs to the orchestrator (-> hub/situation.Surface)", n, word)
			}
		}
	}
}

// orchestratorNouns are what the harness may not name. A method naming one is a rule about work
// reaching into the box: the harness carries out what it is given and reports the outcome.
var orchestratorNouns = []string{"task", "pr", "review", "verdict", "backlog", "feature", "subtask"}

// TestTheHarnessNamesNoWork reads workflow.Harness itself and checks every method and parameter.
func TestTheHarnessNamesNoWork(t *testing.T) {
	iface := interfaceNamed(t, filepath.Join(moduleRoot(t), "internal", "hub", "workflow", "engine.go"), "Harness")
	if iface == nil {
		t.Fatal("workflow.Harness not found — the guard is reading the wrong file")
	}
	methods := 0
	for _, field := range iface.Methods.List {
		fn, ok := field.Type.(*ast.FuncType)
		if !ok || len(field.Names) == 0 {
			continue
		}
		methods++
		words := []string{field.Names[0].Name}
		for _, p := range fn.Params.List {
			for _, n := range p.Names {
				words = append(words, n.Name)
			}
		}
		for _, w := range words {
			for _, noun := range orchestratorNouns {
				if strings.EqualFold(w, noun) {
					t.Errorf("Harness names %q — the box knows nothing of a %s, and a method that needs "+
						"one belongs on Deps instead", w, noun)
				}
			}
		}
	}
	if methods < 8 {
		t.Fatalf("only %d methods found on Harness — the guard is not reading the interface", methods)
	}
}

// exportedNamesIn is every exported type, field, func and method name declared under dir.
func exportedNamesIn(t *testing.T, dir string) (names []string, files int) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		files++
		f, perr := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, e.Name()), nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", e.Name(), perr)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch d := n.(type) {
			case *ast.TypeSpec:
				names = append(names, d.Name.Name)
			case *ast.FuncDecl:
				names = append(names, d.Name.Name)
			case *ast.Field:
				for _, id := range d.Names {
					names = append(names, id.Name)
				}
			}
			return true
		})
	}
	kept := names[:0]
	for _, n := range names {
		if n != "" && n[0] >= 'A' && n[0] <= 'Z' {
			kept = append(kept, n)
		}
	}
	return kept, files
}

// interfaceNamed finds one interface declaration by name in a file.
func interfaceNamed(t *testing.T, path, want string) *ast.InterfaceType {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var found *ast.InterfaceType
	ast.Inspect(f, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok || spec.Name.Name != want {
			return true
		}
		if iface, isIface := spec.Type.(*ast.InterfaceType); isIface {
			found = iface
		}
		return false
	})
	return found
}
