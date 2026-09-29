// package: arch / flow
// type:    test (architecture invariant)
// job:     fail the build if the workflow's declared states can reach anything that writes — the
// decision half must stay pure, and if the apply half loses a handler for an action a state can ask
// for, which would leave an agent with no answer at all.
// limits:  the seam. Whether a rule is RIGHT is hub/flow's own tests'.
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// flowMayImport is everything hub/flow is allowed to reach. All of it is DATA — the shapes a
// decision reads — plus the engine whose State type it declares itself in. Nothing here can write a
// row, steer a session, read a clock or start a context, which is what makes "decide is pure" a
// property of the package rather than a discipline somebody has to remember.
var flowMayImport = map[string]string{
	"time":        "durations, for how stale an answer may be",
	"fmt":         "rendering a map as text, and naming a role that has no flow",
	"strings":     "rendering a map as text",
	"context":     "the ctx the engine hands an action; it never reaches through one itself",
	"sync":        "the engine's own bookkeeping",
	"sync/atomic": "the engine's correlation ids",

	"github.com/flo-at/sindri/internal/hub/flow":            "the world and the shapes a map is written in",
	"github.com/flo-at/sindri/internal/hub/flow/machine":    "the engine the maps are declared in",
	"github.com/flo-at/sindri/internal/hub/flow/agent/act":  "the actions a map names",
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond": "the conditions a map watches",
	"github.com/flo-at/sindri/internal/hub/flow/agent/says": "what a map has the agent told",
	"github.com/flo-at/sindri/internal/hub/flow/topic":      "the events a condition wakes on",
	"github.com/flo-at/sindri/internal/hub/api/agents/verb": "the verbs a map offers",

	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/lifecycle": "the states every role shares, built once",
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker":    "collected into one registry",
	"github.com/flo-at/sindri/internal/hub/flow/pr":                    "a merge intent's own map",
	"github.com/flo-at/sindri/internal/hub/flow/task":                  "a task's own map",
	"github.com/flo-at/sindri/internal/hub/flow/run":                   "a queued run's own map",
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/planner":   "collected into one registry",
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/reviewer":  "collected into one registry",
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/coauthor":  "collected into one registry",

	"github.com/flo-at/sindri/internal/hub/world/situation": "the gathered world, already assembled",
	"github.com/flo-at/sindri/internal/hub/world/store":     "the task and state SHAPES, never the store itself",
	"github.com/flo-at/sindri/internal/api":                 "wire types and pure board rules",
}

// TestTheDeciderCanReachNothingThatWrites walks hub/flow's imports. Purity was a comment on
// directive() and the comment was wrong: inside one call it healed the tree, wrote state three times,
// fired a clear and claimed work. A package that cannot reach a writer cannot drift back.
func TestTheDeciderCanReachNothingThatWrites(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "internal", "hub", "flow")
	seen := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if isActing(path) {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		seen++
		rel, _ := filepath.Rel(moduleRoot(t), path)
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if _, ok := flowMayImport[p]; !ok {
				t.Errorf("%s imports %q — the flow tree declares and decides, so an import that can "+
					"write a row, steer a session or reach an adapter does not belong in it",
					filepath.ToSlash(rel), p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if seen < 18 {
		t.Fatalf("only %d files scanned under hub/flow — the scan is not reading the tree", seen)
	}
}

// TestNoStateReachesAReceiver is the sharper half: a state's Decide is a plain function over a world,
// so nothing in the package may hang off a type that owns anything. A method here would be the seam
// through which the store came back.
func TestNoMapReachesAReceiver(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "internal", "hub", "flow")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if isActing(path) {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			return perr
		}
		rel, _ := filepath.Rel(moduleRoot(t), path)
		for _, dec := range f.Decls {
			fn, ok := dec.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}
			t.Errorf("%s declares %s on %s — a map is data, and a receiver is where a handle to "+
				"something writable comes back", filepath.ToSlash(rel), fn.Name.Name, receiverType(fn))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

// TestNoTwoSubjectsShareAStateName walks every declared map. The subjects are separate machines over
// separate worlds, so two of them could name a state alike without the compiler minding — and then a
// record saying "moved to open" would answer for either. The prefix is what keeps a pass readable
// across four flows.
func TestNoTwoSubjectsShareAStateName(t *testing.T) {
	owner := map[string]string{}
	for _, m := range []struct {
		subject string
		names   []string
	}{
		{"agent", stateNames(t, "agent", "roles")},
		{"pr", stateNames(t, "pr")},
		{"task", stateNames(t, "task")},
		{"run", stateNames(t, "run")},
	} {
		for _, n := range m.names {
			if was, dup := owner[n]; dup {
				t.Errorf("state %q is declared by both %s and %s", n, was, m.subject)
			}
			owner[n] = m.subject
		}
	}
	if len(owner) < 30 {
		t.Fatalf("only %d state names found across the flows — the scan is not reading them", len(owner))
	}
}

// stateNames reads the state-name constants a subject's package declares, from the source rather
// than by importing it: internal/arch stays a guard over the tree, never a consumer of it.
func stateNames(t *testing.T, subject ...string) []string {
	t.Helper()
	root := filepath.Join(append([]string{moduleRoot(t), "internal", "hub", "flow"}, subject...)...)
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return walkErr
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for _, m := range statePattern.FindAllStringSubmatch(string(src), -1) {
			out = append(out, m[1])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", subject, err)
	}
	return out
}

// statePattern finds a declared state name: a constant whose value is a "subject/state" string.
var statePattern = regexp.MustCompile(`=\s*"([a-z]+/[a-z-]+)"`)

// isActing reports the files under hub/flow that are the ACTING half, and so may hold a receiver
// and reach something writable. Three shapes, because the line is drawn wherever a map is not:
//
//   - *_act.go, for a subject that keeps its map and its acting half in ONE directory (flow/pr,
//     flow/task, flow/run). There the suffix is the whole distinction.
//   - flow/agent/*.go and flow/agent/workspace/*.go: the agent's maps live further DOWN
//     (flow/agent/roles/<role>/), and its vocabulary sits beside them in its own packages, so every
//     file at these two levels already acts and a suffix would say nothing.
//   - flow/machine and flow/fleet: the engine, and the assembly that runs one machine per subject.
func isActing(path string) bool {
	p := filepath.ToSlash(path)
	if strings.HasSuffix(p, "_act.go") {
		return true
	}
	if strings.Contains(p, "/flow/machine/") || strings.Contains(p, "/flow/fleet/") {
		return true
	}
	// These two levels only: a role's own map sits in flow/agent/roles/<role>/, and act, cond and
	// says are its vocabulary — all of them stay declarations.
	dir, _ := filepath.Split(p)
	return strings.HasSuffix(dir, "/flow/agent/") || strings.HasSuffix(dir, "/flow/agent/workspace/")
}
