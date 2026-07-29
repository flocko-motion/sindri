package lint

import (
	"bytes"
	"strings"
	"testing"
)

// TestJSLintSilentWithoutJSSource: a Go-only repo hears nothing from this check.
func TestJSLintSilentWithoutJSSource(t *testing.T) {
	root := writeTree(t, map[string]string{"main.go": "package main\n"})
	var out bytes.Buffer
	found, err := JSLint(root, mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if found || out.Len() > 0 {
		t.Errorf("nothing to say about a repo with no JS/TS, got: %q", out.String())
	}
}

// TestJSLintFindsProjectsInSubdirs: a frontend usually lives in web/ or ui/, and a monorepo has
// a project per package — so discovery cannot assume the repo root. Source is attributed to the
// nearest project above it, the config a compiler would resolve.
func TestJSLintFindsProjectsInSubdirs(t *testing.T) {
	root := writeTree(t, map[string]string{
		"web/package.json":          `{"name":"web"}`,
		"web/tsconfig.json":         `{"compilerOptions":{}}`,
		"web/src/a.ts":              "export const a = 1;\n",
		"packages/lib/package.json": `{"name":"lib"}`, // declared, but nothing checks it
		"packages/lib/src/b.ts":     "export const b = 2;\n",
		"stray/src/c.ts":            "export const c = 3;\n", // no config anywhere above
	})
	var out bytes.Buffer
	found, err := JSLint(root, mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("expected findings, got none:\n%s", out.String())
	}
	got := out.String()
	// The unconfigured tree, named by its directory rather than file by file.
	if !strings.Contains(got, "stray") {
		t.Errorf("source with no config above it must be reported:\n%s", got)
	}
	// A package.json with no checker configured is itself the finding.
	if !strings.Contains(got, "packages/lib") {
		t.Errorf("a declared project with no tsconfig/eslint must be reported:\n%s", got)
	}
	// web/ has a tsconfig, so it is delegated to (and reports tsc missing in this environment).
	if !strings.Contains(got, "web") {
		t.Errorf("the configured project should be acted on:\n%s", got)
	}
}

// TestJSLintIgnoresAgentWorktrees: the hub checks agent worktrees out under .worktrees/, each a
// full copy of the repo. Descending into them would lint the same sources many times and report
// another agent's in-progress state as this checkout's problem.
func TestJSLintIgnoresAgentWorktrees(t *testing.T) {
	root := writeTree(t, map[string]string{
		"main.go":                                  "package main\n",
		".worktrees/eitri/web/tsconfig.json":       `{"compilerOptions":{}}`,
		".worktrees/eitri/web/src/d.ts":            "export const d = 4;\n",
		".sindri/claude/eitri/plugins/x/server.ts": "export const e = 5;\n", // vendored agent home
		"node_modules/dep/index.js":                "module.exports = 1;\n",
		"dist/bundle.js":                           "var x=1;\n",
	})
	var out bytes.Buffer
	found, err := JSLint(root, mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if found || out.Len() > 0 {
		t.Errorf("worktrees, agent homes, node_modules and build output are not this repo's source:\n%s", out.String())
	}
}

// TestSkipDirsCoverTheHubsOwnTrees names the directories that must stay out of every linter, so
// removing one is a deliberate act rather than an accident.
func TestSkipDirsCoverTheHubsOwnTrees(t *testing.T) {
	for _, d := range []string{".git", ".worktrees", ".sindri", ".todos", "vendor", "node_modules", "dist", "build", "coverage"} {
		if !skipDirs[d] {
			t.Errorf("%s must be skipped by the linters", d)
		}
	}
}
