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
	found, err := Deadcode(patterns, "", false, &sb)
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

func TestDeadcodeNoMainPackage(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"go.mod":     "module libonly\n\ngo 1.25\n",
		"lib/lib.go": "package lib\n\nfunc Exported() {}\n",
	})

	if _, _, err := func() (string, bool, error) {
		orig, _ := os.Getwd()
		_ = os.Chdir(dir)
		defer os.Chdir(orig)
		var sb strings.Builder
		f, e := Deadcode([]string{"./..."}, "", false, &sb)
		return sb.String(), f, e
	}(); err == nil {
		t.Fatal("expected an error when no main package is present")
	}
}
