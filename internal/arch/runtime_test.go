// package: arch / runtime
// type:    test (architecture invariant)
// job:     fail the build if any code queries the container runtime, or probes an agent's
// liveness/clients, directly. The watchdog (-> hub/watchdog.go) is the one place
// that polls; everything else reads its standing observation, acts (launch/stop/
// inject are container operations in their own right, not a question), or is an
// explicit, user-requested diagnostic. The call sites that still query either
// port directly are listed here with the reason each is allowed to.
// limits:  the query methods only (RunningContext, ListByLabelContext,
// ListByLabelFresh, ListByLabelCached, Diagnose, MemoryCapacity, Stats for the
// container port; AgentAlive, SessionAliveCtx, Clients, ClientsCtx
// for the agent probe) — actions are out of scope for both. The board-read
// invariant itself (hub.State touches only the store and the watchdog) is
// internal/arch/boardread_test.go's, which walks the whole reachable call graph
// and so is strictly stronger than a fixed method-name check would be here.
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

// runtimeQueryMethods are container.Runtime's read-only, liveness-or-capacity-answering methods —
// the ones a poll means, as opposed to Run/Rm/Exec/EnsureImage and the rest, which DO something and
// so are never in question here (-> the watchdog header's "ACTIONS act").
var runtimeQueryMethods = map[string]bool{
	"RunningContext":     true,
	"ListByLabelContext": true, "ListByLabelFresh": true, "ListByLabelCached": true,
	"Diagnose": true, "MemoryCapacity": true, "Stats": true,
}

// declaredRuntimeQueriers are the call sites allowed to query the container runtime directly, each
// with WHY. A key is either a whole file ("internal/hub/watchdog.go") or one function within it
// ("internal/hub/state.go#AllStats") for a file that also carries a call site with no such excuse.
// Anything else calling a query method must read the watchdog's observation instead
// (hub.watchdog.get/.pods, or workflow.Deps' AgentUp/AgentIdle).
var declaredRuntimeQueriers = map[string]string{
	// The observer itself: the one place a listing or a capacity sample is taken, on its own cadence.
	"internal/hub/watchdog.go": "the observer — the one place a listing or a capacity sample is taken",
	// The shared liveness/clients probe mechanism the watchdog's own probe() calls through, and that
	// explicit diagnostics (`agent info`) read too — the probe lives here once, not at each caller.
	"internal/hub/agent/runtime.go": "SessionAliveCtx/Diagnose — the probe mechanism itself",
	// Launch/stop/relaunch act on a specific pod and need to know, right then, whether it is up
	// before doing so — the same immediacy an action gets elsewhere (-> injection's declared files).
	"internal/hub/agent/lifecycle.go": "launch/stop actions and their own wait/debug diagnostics",
	// Inject and Interrupt check a pod is up immediately before writing to it — declared push-only
	// in delivery_test.go for the same reason: the check is part of the action, not a poll.
	"internal/hub/agent/inject.go": "checked immediately before injecting/interrupting, not on a schedule",
	// `agent stats`: an explicit, user-requested snapshot, never a per-render board probe. Scoped to
	// the function alone — state.go's board read (State) must carry none of these (-> boardread_test.go).
	"internal/hub/state.go#AllStats": "AllStats — the explicit `agent stats` diagnostic",
}

// agentProbeMethods are agent.Service's own liveness/clients probes — a container exec apiece, and
// the second half of the invariant delivery didn't cover: the outage this ticket answers reached one
// of these (AgentAlive, from CurrentModel) rather than the container port directly.
var agentProbeMethods = map[string]bool{
	"AgentAlive": true, "SessionAliveCtx": true,
	"Clients": true, "ClientsCtx": true,
}

// declaredAgentProbers are the call sites allowed to probe an agent directly, each with WHY. Same
// key shape as declaredRuntimeQueriers. Anything else must read the watchdog's observation instead
// (agent.Deps' AgentUp/AgentClients, or workflow.Deps' AgentUp/AgentIdle).
var declaredAgentProbers = map[string]string{
	"internal/hub/watchdog.go":      "the observer's own probe",
	"internal/hub/agent/runtime.go": "the probe mechanism itself — these methods call each other here",
	"internal/hub/agent/lifecycle.go": "the launch-wait loop — a bounded one-shot poll for a pod it " +
		"just started, not a tick",
	// Compact and SetModel act on the one agent making (or receiving) the request — the assignment
	// gate and an explicit human model change, neither a sweep over the roster.
	"internal/hub/agent/compact.go#Compact": "checked immediately before compacting — the caller's own assignment gate",
	"internal/hub/agent/model.go#SetModel":  "checked immediately before an explicit model change",
	// The workflow.Deps/agent.Deps seam itself: AgentAlive here IS the pass-through
	// workflow.Deps.AgentAlive is documented to be, not a caller of it.
	"internal/hub/wiring.go": "the workflow.Deps/agent.Deps seam — AgentAlive here is the wiring, not a caller",
	// Each below acts on ONE named agent a specific request already identified — never a roster sweep
	// (-> workflow.Deps.AgentAlive's own doc: "for a caller needing the answer as of now, never a tick").
	"internal/hub/workflow/planner.go": "a plan assignment / an edit note, each to the one agent named",
	"internal/hub/workflow/task.go":    "UnassignTask, checked against the one agent holding the task",
	"internal/hub/workflow/scrap.go":   "ScrapPR, checked against the one reviewer holding the review",
	// Explicit, user-requested diagnostics: `agent info`'s HTTP endpoints, the CLI command itself, and
	// the TUI's detail-view fetch — each fired once per invocation, never on a tick or a render.
	"internal/hub/server.go":     "agent info's HTTP endpoints — explicit per-request diagnostics",
	"internal/ui/cli/agent.go":   "`agent info` — an explicit CLI diagnostic",
	"internal/ui/tui/refresh.go": "the TUI detail view's fetch, user-driven",
}

// probeGuard walks the module looking for calls whose selector name is in methods, failing any call
// site whose (file, enclosing function) is not in declared (checked at the function first, the whole
// file second). Shared by the container-port and the agent-probe guards below — same shape, same
// declare-it-or-fix-it rule, different vocabulary of methods.
func probeGuard(t *testing.T, methods map[string]bool, declared map[string]string, packageQualified bool) (seen int, undeclared []string) {
	t.Helper()
	root := moduleRoot(t)
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
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			return nil // not parseable is brokkr lint's business, not this guard's
		}
		relSlash := filepath.ToSlash(rel)
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			funcName := fn.Name.Name
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || !methods[sel.Sel.Name] {
					return true
				}
				if packageQualified {
					// container.RunningContext(...): a package-qualified call, not any type's same-named
					// method — a local variable named "container" (state.go has one, a pod's name)
					// is not this call.
					if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "container" {
						return true
					}
				}
				seen++
				_, byFunc := declared[relSlash+"#"+funcName]
				_, byFile := declared[relSlash]
				if !byFunc && !byFile {
					undeclared = append(undeclared, rel+" ("+funcName+"): "+sel.Sel.Name)
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return seen, undeclared
}

// TestNothingPollsTheRuntimeWithoutDeclaringWhy walks the module and finds every call into one of
// the container port's query methods. Each must sit in a declared file or function; anything else
// is a fresh probe on a read or tick path — the exact cost the watchdog exists to spare.
func TestNothingPollsTheRuntimeWithoutDeclaringWhy(t *testing.T) {
	seen, undeclared := probeGuard(t, runtimeQueryMethods, declaredRuntimeQueriers, true)
	// Anti-vacuity: the files on the list are real call sites, so a scan finding too few is a broken
	// scan (an import alias, a moved package) rather than a suddenly well-behaved tree.
	if seen < 10 {
		t.Fatalf("only %d runtime query call sites found — the scan is not reading the module", seen)
	}
	for _, u := range undeclared {
		t.Errorf("%s queries the container runtime directly — read the watchdog's observation instead "+
			"(hub.watchdog.get/.pods, or workflow.Deps' AgentUp/AgentIdle), or add it to "+
			"declaredRuntimeQueriers with the reason it needs a fresh reading", u)
	}
}

// TestNothingProbesAnAgentWithoutDeclaringWhy is the other half: a container operation reached
// THROUGH agent.Service's own AgentAlive/Clients methods is invisible to the guard above, which
// only matches container.X calls — and that is exactly the shape of the outage this ticket answers
// (State -> CurrentModel -> AgentAlive -> container.RunningContext), plus the two still live when
// this was written: FireIdleStops and FireArmedClears, both driven off the hub's own roster-wide
// tick (-> hub/refwatch.go), each probing every member it reaches rather than reading AgentUp.
func TestNothingProbesAnAgentWithoutDeclaringWhy(t *testing.T) {
	seen, undeclared := probeGuard(t, agentProbeMethods, declaredAgentProbers, false)
	if seen < 10 {
		t.Fatalf("only %d agent probe call sites found — the scan is not reading the module", seen)
	}
	for _, u := range undeclared {
		t.Errorf("%s probes an agent directly — read the watchdog's observation instead "+
			"(agent.Deps' AgentUp/AgentClients, or workflow.Deps' AgentUp/AgentIdle), or add it to "+
			"declaredAgentProbers with the reason it needs a fresh reading", u)
	}
}
