package container

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFailureDetail(t *testing.T) {
	// A connection failure (the macOS "machine not started" case) surfaces the
	// message AND the podman-machine hint.
	got := buildFailureDetail("Error: Cannot connect to Podman socket")
	if !strings.Contains(got, "Cannot connect") || !strings.Contains(got, "podman machine start") {
		t.Errorf("connection failure should surface the error + hint, got:\n%s", got)
	}
	// Empty output still yields the hint (better than nothing).
	if got := buildFailureDetail(""); !strings.Contains(got, "podman machine") {
		t.Errorf("empty output should fall back to the hint, got: %q", got)
	}
	// An ordinary build error (no connection signal) is surfaced without the hint.
	got = buildFailureDetail("Step 3/9: RUN apt-get install foo\nE: Unable to locate package foo")
	if !strings.Contains(got, "Unable to locate package foo") {
		t.Errorf("build error should be surfaced, got:\n%s", got)
	}
	if strings.Contains(got, "podman machine") {
		t.Errorf("a normal build error must not get the machine hint, got:\n%s", got)
	}
	// Long output is tailed to the last lines (where the error lands).
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString("layer line\n")
	}
	b.WriteString("Error: final failure line")
	got = buildFailureDetail(b.String())
	if !strings.Contains(got, "final failure line") {
		t.Errorf("tail must include the final error line, got:\n%s", got)
	}
	if strings.Count(got, "\n") > 12 {
		t.Errorf("tail should be bounded (~12 lines), got %d lines", strings.Count(got, "\n"))
	}
}

// The build context must be embedded so an installed binary can build the agent
// image for any repo — the files the Dockerfile COPYs have to be present.
func TestEmbeddedBuildContextHasRecipe(t *testing.T) {
	want := []string{
		"buildctx/Dockerfile",
		"buildctx/sindri-agent.sh",
		"buildctx/yazi.sh",
		"buildctx/shims/docker",
		"buildctx/shims/docker-compose",
	}
	for _, p := range want {
		if _, err := buildContext.ReadFile(p); err != nil {
			t.Errorf("embedded build context missing %s: %v", p, err)
		}
	}
}

// materialize must write the embedded tree to disk with the Dockerfile at the
// context root (the "buildctx/" prefix stripped) and the shims under shims/.
func TestMaterializeStripsPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := materialize(dir); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	for _, rel := range []string{"Dockerfile", "sindri-agent.sh", "shims/docker"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("materialized context missing %s: %v", rel, err)
		}
	}
	// And nothing should remain under a nested buildctx/ dir.
	if _, err := os.Stat(filepath.Join(dir, "buildctx")); !os.IsNotExist(err) {
		t.Errorf("buildctx/ prefix not stripped (got err %v)", err)
	}
}

// materialize must overwrite a stale staging dir cleanly (a removed recipe file
// must not survive into the next build).
func TestMaterializeClearsStaleStaging(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "stale-file")
	if err := os.WriteFile(stale, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := materialize(dir); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale file survived materialize (err %v)", err)
	}
	// Sanity: the Dockerfile is readable as a regular file.
	if err := fs.WalkDir(os.DirFS(dir), "Dockerfile", func(_ string, d fs.DirEntry, err error) error {
		return err
	}); err != nil {
		t.Errorf("Dockerfile not present after materialize: %v", err)
	}
}
