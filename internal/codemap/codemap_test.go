package codemap

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree lays out .go files (relative path -> contents) under a fresh dir and
// returns that dir.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, src := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestWriteSingleRootLabelsRelativeToRoot(t *testing.T) {
	root := writeTree(t, map[string]string{"pkg/a.go": "package pkg\n"})
	var buf bytes.Buffer
	if err := Write(&buf, []string{filepath.Join(root, "pkg")}, -1, "", ""); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "a.go") || strings.Contains(got, "pkg/a.go") {
		t.Fatalf("single root should label concisely as a.go, got:\n%s", got)
	}
}

func TestWriteMultiRootLabelsAreUnambiguous(t *testing.T) {
	root := writeTree(t, map[string]string{
		"one/a.go": "package one\n",
		"two/a.go": "package two\n",
	})
	// Run from root so cwd-relative labels carry the directory prefix.
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, []string{"one", "two"}, -1, "", ""); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "one/a.go") || !strings.Contains(got, "two/a.go") {
		t.Fatalf("multiple roots should label files with their directory, got:\n%s", got)
	}
}

func TestWriteMissingRootFailsLoud(t *testing.T) {
	root := writeTree(t, map[string]string{"ok/a.go": "package ok\n"})
	var buf bytes.Buffer
	err := Write(&buf, []string{filepath.Join(root, "ok"), filepath.Join(root, "nope")}, -1, "", "")
	if err == nil {
		t.Fatal("a missing root must be a loud error, not a silent skip")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Fatalf("error must name the offending path, got: %v", err)
	}
	// Nothing should have been emitted before the bad root was caught.
	if buf.Len() != 0 {
		t.Fatalf("must validate roots before emitting any output, got:\n%s", buf.String())
	}
}
