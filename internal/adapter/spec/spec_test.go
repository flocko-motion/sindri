package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateNoOpenspecDirSkipsSilently(t *testing.T) {
	ok, out := Validate(t.TempDir())
	if !ok || out != "" {
		t.Fatalf("a project without openspec/ should skip silently, got ok=%v out=%q", ok, out)
	}
}

func TestValidateMissingCLIDegradesVisibly(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "openspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "") // hide the openspec CLI
	ok, out := Validate(dir)
	if !ok {
		t.Fatal("a missing optional CLI must not be a validation failure")
	}
	if !strings.Contains(out, "not installed") {
		t.Errorf("the skip must be visible, got: %q", out)
	}
}
