// package: arch / situation
// type:    test (architecture invariant)
// job:     fail the build when a rule the surface owns is derived anywhere else, and when the
// gathering behind it could cost a runtime call — the two halves of "one surface says what is
// allowed" (-> hub/situation).
// limits:  the call sites and the imports. Whether a rule is RIGHT is the surface's own tests'.
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

// ownedRules are the questions the surface answers, keyed by the raw fact a site would reach for to
// answer one itself. Prose has already proved insufficient here: the guard on nudging a retired agent
// was added to one function and omitted from another in the same commit, three lines away.
var ownedRules = map[string]string{
	"Retired":    "whether a retired agent may be handed or told anything — ask Surface.Assign/Nudge/Wake",
	"ClearArmed": "whether an armed clear withholds work — ask Surface.Assign/Nudge/Wake",
}

// ruleDerivers are the files allowed to read those facts, each with why it is not a second decider.
// A file that reads one to RENDER it, or to write it, belongs here; one that reads it to decide what
// may happen to an agent does not, and the surface is where that rule goes.
var ruleDerivers = map[string]string{
	// The surface itself, and the gathering that feeds it.
	"internal/hub/situation/surface.go":   "the surface — the one home for these rules",
	"internal/hub/situation/situation.go": "the gathering that feeds it",
	// The store: these are its columns.
	"internal/hub/store/store.go":    "the roster row's own definition",
	"internal/hub/store/agent.go":    "reads and writes those columns",
	"internal/hub/store/reviewer.go": "the reviewer pool's own query, filtering on the column in SQL",
	// The writers: setting a flag is not deriving a rule from it.
	"internal/hub/agent/clearcontext.go": "sets and clears the arming — its writer",
	"internal/hub/agent/retire.go":       "sets and clears retirement — its writer",
	"internal/hub/server.go":             "the retire endpoint's own request field, not a roster row",
	"internal/hub/hub.go":                "SetRetired reads the prior value to spot a return to service",
	// Rendering: the board carries these as fields for a front-end to show.
	"internal/hub/state.go":    "projects the roster onto the board, flags included",
	"internal/hub/commands.go": "renders the retirement note beside what the agent holds",
	// Read off the SITUATION, which is the sanctioned carrier — the fact, for a caller that needs the
	// fact rather than a rule: which armed clear to fire, and which directive a retired agent gets.
	"internal/hub/workflow/engine.go": "reads ClearArmed off the situation, to fire the clear it names",
	"internal/hub/workflow/task.go":   "reads Retired off the situation, to serve DirRetired",
	// The front-ends render them; a rule they applied themselves is what AgentView.NeedsUser exists
	// to prevent, and importguard_test.go keeps them from reaching the surface anyway.
	"internal/ui":  "front-ends render the flags they are given",
	"internal/api": "wire types and the pure board rules the hub itself asks",
}

// TestTheSurfaceIsTheOnlyHomeForTheseRules walks the module for a site that reads one of those facts
// off a roster row and is not on the list — the sixth decider the change exists to refuse.
func TestTheSurfaceIsTheOnlyHomeForTheseRules(t *testing.T) {
	root := moduleRoot(t)
	var problems []string
	seen := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
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
			return nil // unparseable is brokkr lint's business, not this guard's
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			why, owned := ownedRules[sel.Sel.Name]
			if !owned {
				return true
			}
			seen++
			if allowedDeriver(rel) {
				return true
			}
			problems = append(problems, rel+" reads ."+sel.Sel.Name+" to decide "+why)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	// Anti-vacuity: these columns are read in the store, the surface and the board at the very least,
	// so a scan finding almost none is a broken scan rather than a clean tree.
	if seen < 6 {
		t.Fatalf("only %d reads of the owned facts found across the module — the scan is not reading it", seen)
	}
	sort.Strings(problems)
	for _, p := range dedupe(problems) {
		t.Errorf("%s.\nAsk the surface instead (situation.Situation.Allowed), or add the file to "+
			"ruleDerivers with the reason it is not a second decider", p)
	}
}

// allowedDeriver reports whether rel is declared, by exact path or by a declared directory prefix.
func allowedDeriver(rel string) bool {
	for p := range ruleDerivers {
		if rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}

// TestASituationCostsNoRuntimeCall holds the other half: gathering must stay free of anything that
// spawns, dials or reads a transcript, whatever an agent's state. The situation sits on the board's
// own path, so a query added here is paid per agent per render, times the connected clients.
func TestASituationCostsNoRuntimeCall(t *testing.T) {
	dir := filepath.Join(moduleRoot(t), "internal", "hub", "situation")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	files := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		files++
		f, perr := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, e.Name()), nil, parser.ImportsOnly)
		if perr != nil {
			t.Fatalf("parse %s: %v", e.Name(), perr)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if why, forbidden := queryingImports[path]; forbidden {
				t.Errorf("%s imports %s — %s. A situation is gathered on the board's own path; take the "+
					"reading on the observer's sweep and hand it over instead (-> situation.Observer)",
					e.Name(), path, why)
			}
		}
	}
	if files < 2 {
		t.Fatalf("only %d source file(s) found in %s — the scan is reading the wrong directory", files, dir)
	}
}
