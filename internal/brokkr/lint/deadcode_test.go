package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeModule lays down a tiny self-contained module and returns its dir.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runInDir(t *testing.T, dir string, patterns ...string) (string, bool) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	var sb strings.Builder
	found, err := Deadcode(patterns, "", nil, nil, &sb)
	if err != nil {
		t.Fatalf("Deadcode: %v", err)
	}
	return sb.String(), found
}

func TestDeadcodeReportsUnreachable(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"go.mod": "module deadprog\n\ngo 1.25\n",
		"main.go": `package main

func main() { live() }

func live() {}

func dead() {}

type T struct{}

func (T) Reachable()   { live() }
func (T) Unreachable() {}
`,
	})

	out, found := runInDir(t, dir, "./...")
	if !found {
		t.Fatalf("expected dead code to be found; output:\n%s", out)
	}
	if !strings.Contains(out, "unreachable func: dead") {
		t.Errorf("expected 'dead' to be reported; got:\n%s", out)
	}
	if !strings.Contains(out, "unreachable func: T.Unreachable") {
		t.Errorf("expected 'T.Unreachable' to be reported; got:\n%s", out)
	}
	if strings.Contains(out, "unreachable func: live") {
		t.Errorf("'live' is reachable and must not be reported; got:\n%s", out)
	}
	if strings.Contains(out, "func: main") {
		t.Errorf("'main' is a root and must not be reported; got:\n%s", out)
	}
}

func TestDeadcodeKeepDirective(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"go.mod": "module keepprog\n\ngo 1.25\n",
		"main.go": `package main

func main() { live() }

func live() {}

//deadcode:keep
func keptDead() {}

func reallyDead() {}
`,
	})

	out, found := runInDir(t, dir, "./...")
	if !found {
		t.Fatalf("expected reallyDead to be reported; output:\n%s", out)
	}
	if strings.Contains(out, "keptDead") {
		t.Errorf("keptDead carries //deadcode:keep and must not be reported; got:\n%s", out)
	}
	if !strings.Contains(out, "unreachable func: reallyDead") {
		t.Errorf("expected reallyDead to be reported; got:\n%s", out)
	}
	if !strings.Contains(out, "//deadcode:keep") {
		t.Errorf("expected the output to advertise the //deadcode:keep directive; got:\n%s", out)
	}
}

func TestDeadcodeCleanProgram(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"go.mod":  "module cleanprog\n\ngo 1.25\n",
		"main.go": "package main\n\nfunc main() { helper() }\n\nfunc helper() {}\n",
	})

	out, found := runInDir(t, dir, "./...")
	if found || out != "" {
		t.Fatalf("expected no dead code; got found=%v output:\n%s", found, out)
	}
}

func TestDeadcodeSkipsWithoutGoToolchain(t *testing.T) {
	t.Setenv("PATH", "") // hide the go toolchain
	var sb strings.Builder
	found, err := Deadcode([]string{"./..."}, "", nil, nil, &sb)
	if err != nil {
		t.Fatalf("a missing go toolchain must degrade, not error: %v", err)
	}
	if found {
		t.Error("a skip must report nothing found")
	}
	if !strings.Contains(sb.String(), "skipping") {
		t.Errorf("the skip must be visible, got: %q", sb.String())
	}
}

// TestDeadcodeSkipsALibraryModule: reachability is traced FROM main packages, so a module with none
// gives the analysis nothing to say. It exited 1 with "no main packages among [./...]", which reads
// as a finding about the code and failed a gate over a perfectly valid package shape — a library, or
// any subdirectory scoped without a main. Skipped and said, as a non-Go tree already is.
func TestDeadcodeSkipsALibraryModule(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"go.mod":     "module libonly\n\ngo 1.25\n",
		"lib/lib.go": "package lib\n\nfunc Exported() {}\n",
	})

	out, found, err := func() (string, bool, error) {
		orig, _ := os.Getwd()
		_ = os.Chdir(dir)
		defer os.Chdir(orig)
		var sb strings.Builder
		f, e := Deadcode([]string{"./..."}, "", nil, nil, &sb)
		return sb.String(), f, e
	}()
	if err != nil {
		t.Fatalf("a library module must not fail the linter: %v", err)
	}
	if found {
		t.Error("nothing can be unreachable when there is nothing to be reachable from")
	}
	if !strings.Contains(out, "skipping") {
		t.Errorf("the skip must be visible, or the linter looks like it ran: %q", out)
	}
}

// TestDeadcodeFindsAModuleThatIsNotAtTheRoot is the report from a worker whose repo keeps its Go
// code in a subdirectory. Loading "./..." from the root failed with "directory prefix . does not
// contain main module or its selected dependencies", so the gate went red — and the worker started
// restructuring the repository to satisfy the linter. Go is happy with a module anywhere; a linter
// that assumes one at the top is dictating layout rather than reporting on code.
func TestDeadcodeFindsAModuleThatIsNotAtTheRoot(t *testing.T) {
	root := t.TempDir()
	mod := filepath.Join(root, "tools", "templater")
	if err := os.MkdirAll(mod, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(mod, "go.mod"), "module example.com/templater\n\ngo 1.25\n")
	write(t, filepath.Join(mod, "main.go"), "package main\n\nfunc main() { helper() }\n\nfunc helper() {}\n\nfunc unused() {}\n")
	write(t, filepath.Join(root, "README.md"), "no go.mod here\n")

	var sb strings.Builder
	found, err := inDir(t, root, func() (bool, error) { return Deadcode([]string{"./..."}, "", nil, nil, &sb) })
	if err != nil {
		t.Fatalf("a module below the root must be analysed where it lives: %v (%s)", err, sb.String())
	}
	if !found || !strings.Contains(sb.String(), "unused") {
		t.Errorf("the nested module's dead function should be reported:\n%s", sb.String())
	}
}

// TestDeadcodeSkipsATreeWithNoModule: Go files and no go.mod defines nothing to analyse, so it is a
// stated skip — the same reasoning as a TypeScript-only tree, and not a failure of the gate.
func TestDeadcodeSkipsATreeWithNoModule(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "x.go"), "package x\n\nfunc F() {}\n")

	var sb strings.Builder
	found, err := inDir(t, root, func() (bool, error) { return Deadcode([]string{"./..."}, "", nil, nil, &sb) })
	if err != nil {
		t.Fatalf("a tree with no module is a skip, not an error: %v", err)
	}
	if found {
		t.Error("nothing can be found where nothing defines a module")
	}
	if !strings.Contains(sb.String(), "no go.mod") {
		t.Errorf("the skip must say why:\n%s", sb.String())
	}
}

// inDir runs fn with the process in dir, restoring the old one. The linter loads relative to the
// working directory, which is what these two cases are about.
func inDir(t *testing.T, dir string, fn func() (bool, error)) (bool, error) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()
	return fn()
}

// write is a fatal-on-error file write for these fixtures.
func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
