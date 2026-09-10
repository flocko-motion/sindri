package goplsmcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tree lays out directories and files under a temp root: keys are paths, values file contents.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for path, body := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestModuleRootFindsWhatTheRootStatMissed is the reason the launch-time condition was wrong. A
// go.mod at the workspace root was the only shape it recognised, so every one of these read as "not
// a Go project" and the agent lost its Go tools entirely.
func TestModuleRootFindsWhatTheRootStatMissed(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		files      map[string]string
	}{
		{"module at the root", ".", map[string]string{"go.mod": "module x\n"}},
		{"a workspace of several modules", ".", map[string]string{
			"go.work": "go 1.26\n", "a/go.mod": "module a\n", "b/go.mod": "module b\n",
		}},
		{"one module in a subdirectory", "svc", map[string]string{"svc/go.mod": "module svc\n"}},
		{"a mixed-language tree", "backend", map[string]string{
			"package.json": "{}", "backend/go.mod": "module backend\n",
		}},
		{"deeper than one level", "tools/gen", map[string]string{"tools/gen/go.mod": "module gen\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := tree(t, tc.files)
			got, found := ModuleRoot(root)
			if !found {
				t.Fatalf("no module found in %v", tc.files)
			}
			if want := filepath.Join(root, tc.want); filepath.Clean(got) != filepath.Clean(want) {
				t.Errorf("ModuleRoot = %q, want %q", got, want)
			}
		})
	}
}

// TestTheOutermostModuleWins: a breadth-first search, so a repo whose root module also contains a
// nested one is served from the root rather than from whichever the walk reached first.
func TestTheOutermostModuleWins(t *testing.T) {
	root := tree(t, map[string]string{"go.mod": "module x\n", "sub/go.mod": "module sub\n"})
	got, _ := ModuleRoot(root)
	if filepath.Clean(got) != filepath.Clean(root) {
		t.Errorf("ModuleRoot = %q, want the root %q", got, root)
	}
}

// TestNoModuleIsSaidOutLoud is the counterpart obligation of declaring the server everywhere: with
// no condition at launch, this is the only thing that can explain a tree it cannot serve. Silence
// or an empty result would read as a fact about the code.
func TestNoModuleIsSaidOutLoud(t *testing.T) {
	root := tree(t, map[string]string{"README.md": "no go here\n"})
	if got, found := ModuleRoot(root); found {
		t.Fatalf("a tree with no module reported %q", got)
	}
	advice := NoModuleAdvice(root)
	for _, want := range []string{root, "go.mod", "says nothing about the code"} {
		if !strings.Contains(advice, want) {
			t.Errorf("the refusal is missing %q:\n%s", want, advice)
		}
	}
	// An empty workspace is its own case, and must not claim to have examined anything.
	if _, found := ModuleRoot(""); found {
		t.Error("an empty workspace reported a module")
	}
	if !strings.Contains(NoModuleAdvice(""), "nothing was examined") {
		t.Errorf("an empty workspace should say nothing was examined:\n%s", NoModuleAdvice(""))
	}
}

// TestTheSearchDoesNotWanderIntoDependencies: node_modules and vendor hold thousands of directories
// and none of the module being served. A shim that walked them would stall the agent's first call.
func TestTheSearchDoesNotWanderIntoDependencies(t *testing.T) {
	root := tree(t, map[string]string{
		"node_modules/pkg/go.mod": "module vendored\n",
		"vendor/dep/go.mod":       "module vendored\n",
		".git/hooks/go.mod":       "module vendored\n",
	})
	if got, found := ModuleRoot(root); found {
		t.Errorf("the search descended into a skipped directory: %q", got)
	}
}
