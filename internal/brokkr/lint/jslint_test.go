package lint

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
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

// subdirTree is the discovery fixture: a frontend in web/, a monorepo package, and stray source.
func subdirTree(t *testing.T) string {
	t.Helper()
	return writeTree(t, map[string]string{
		"web/package.json":          `{"name":"web"}`,
		"web/tsconfig.json":         `{"compilerOptions":{}}`,
		"web/src/a.ts":              "export const a = 1;\n",
		"packages/lib/package.json": `{"name":"lib"}`, // declared, but nothing checks it
		"packages/lib/src/b.ts":     "export const b = 2;\n",
		"stray/src/c.ts":            "export const c = 3;\n", // no config anywhere above
	})
}

// stubToolPath puts an executable `name` that exits with code on a PATH containing nothing else, so
// the delegated tool's presence is the test's choice rather than the machine's.
func stubToolPath(t *testing.T, name string, code int) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nexit " + strconv.Itoa(code) + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// TestJSLintFindsProjectsInSubdirs: a frontend usually lives in web/ or ui/, and a monorepo has
// a project per package — so discovery cannot assume the repo root. Source is attributed to the
// nearest project above it, the config a compiler would resolve.
//
// The delegated tool is stubbed rather than inherited from the machine. Asserting on "tsc is not
// installed here" passed only where it was absent, so this failed in CI, which has it.
func TestJSLintFindsProjectsInSubdirs(t *testing.T) {
	// Findings that hold however the toolchain is provisioned: discovery itself.
	assertDiscovery := func(t *testing.T, got string) {
		t.Helper()
		// The unconfigured tree, named by its directory rather than file by file.
		if !strings.Contains(got, "stray") {
			t.Errorf("source with no config above it must be reported:\n%s", got)
		}
		// A package.json with no checker configured is itself the finding.
		if !strings.Contains(got, "packages/lib") {
			t.Errorf("a declared project with no tsconfig/eslint must be reported:\n%s", got)
		}
	}

	t.Run("tool missing is itself a finding", func(t *testing.T) {
		stubToolPath(t, "unrelated", 0) // a PATH with no tsc on it
		root := subdirTree(t)
		var out bytes.Buffer
		found, err := JSLint(root, mustIgnore(t), &out)
		if err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if !found {
			t.Fatalf("expected findings, got none:\n%s", got)
		}
		assertDiscovery(t, got)
		// web/ configures tsc, so a missing tsc is a broken promise, not a pass.
		if !strings.Contains(got, "web") || !strings.Contains(got, "not installed") {
			t.Errorf("a configured project whose tool is absent must be reported:\n%s", got)
		}
		// This PATH has no node either, so the advice must say so — `npm install` cannot help.
		if !strings.Contains(got, "node itself is not on PATH") {
			t.Errorf("with no node, the advice must name that rather than suggest npm install:\n%s", got)
		}
	})

	t.Run("tool present and clean is silent about that project", func(t *testing.T) {
		stubToolPath(t, "tsc", 0) // a tsc that type-checks everything happily
		root := subdirTree(t)
		var out bytes.Buffer
		found, err := JSLint(root, mustIgnore(t), &out)
		if err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if !found {
			t.Fatalf("stray and packages/lib are still findings:\n%s", got)
		}
		assertDiscovery(t, got)
		if strings.Contains(got, "not installed") {
			t.Errorf("tsc was on PATH, so nothing may claim it is missing:\n%s", got)
		}
	})

	t.Run("tool present and failing is reported", func(t *testing.T) {
		stubToolPath(t, "tsc", 2) // a tsc that rejects the project
		root := subdirTree(t)
		var out bytes.Buffer
		found, err := JSLint(root, mustIgnore(t), &out)
		if err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if !found {
			t.Fatalf("expected findings, got none:\n%s", got)
		}
		if !strings.Contains(got, "web") || !strings.Contains(got, "failed") {
			t.Errorf("a failing type-check must be reported against its project:\n%s", got)
		}
	})
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

// stubTool installs an executable script at rel under root, standing in for a tool from
// node_modules/.bin. writeTree writes 0644, and a tool has to be runnable.
func stubTool(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestJSToolRunsInsideItsProject: a delegated tool must run with its PROJECT as the working
// directory, because that is how tsc and eslint find their own config. It also pins the bug
// behind that: the tool path was relative, and the child resolves a relative argv[0] after
// chdir-ing into the same directory, so "web/node_modules/.bin/tsc" became
// "web/web/node_modules/.bin/tsc" and the tool never started at all.
func TestJSToolRunsInsideItsProject(t *testing.T) {
	root := writeTree(t, map[string]string{
		"web/package.json":  `{"name":"web"}`,
		"web/tsconfig.json": `{"compilerOptions":{}}`,
		"web/src/a.ts":      "export const a = 1;\n",
	})
	// Reports its own cwd and whether the config is visible there, then fails so the report
	// relays it.
	stubTool(t, root, "web/node_modules/.bin/tsc",
		"#!/bin/sh\necho \"cwd=$PWD\"\n[ -f tsconfig.json ] && echo CONFIG-VISIBLE || echo CONFIG-MISSING\nexit 1\n")

	var out bytes.Buffer
	found, err := JSLint(root, mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("the stub exits 1, so this must be a finding:\n%s", out.String())
	}
	got := out.String()
	if !strings.Contains(got, "CONFIG-VISIBLE") {
		t.Errorf("the tool must run where its tsconfig is, got:\n%s", got)
	}
	if !strings.Contains(got, "cwd=") {
		t.Fatalf("the tool did not run — its output was never relayed:\n%s", got)
	}
	// The cwd must be the project, not the repo root the linter was invoked from.
	if !strings.Contains(got, filepath.Join(root, "web")) {
		t.Errorf("expected cwd %s, got:\n%s", filepath.Join(root, "web"), got)
	}
}

// TestJSToolNamesAnExecFailure: a tool that produces NO output failed before it could speak, and
// only the exec error says why. Reporting a bare "failed" sent a reader hunting for a type error
// that was never reported, when the fault was the exec itself.
func TestJSToolNamesAnExecFailure(t *testing.T) {
	root := writeTree(t, map[string]string{
		"web/package.json":          `{"name":"web"}`,
		"web/tsconfig.json":         `{"compilerOptions":{}}`,
		"web/src/a.ts":              "export const a = 1;\n",
		"web/node_modules/.bin/tsc": "#!/bin/sh\nexit 0\n", // present but NOT executable (0644)
	})
	var out bytes.Buffer
	found, err := JSLint(root, mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("an unrunnable tool is a failure:\n%s", out.String())
	}
	if got := out.String(); !strings.Contains(got, "did not run") {
		t.Errorf("a tool that never ran must say so, with the reason:\n%s", got)
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
