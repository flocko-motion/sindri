package container

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

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
