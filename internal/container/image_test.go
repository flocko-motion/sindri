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
		"buildctx/shell.sh",
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

// customDockerfile discovers a user recipe in the central sindri home (StateDir,
// via SINDRI_HOME), preferring Containerfile, and ignores a directory of that name.
func TestCustomDockerfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SINDRI_HOME", home)

	if got := customDockerfile(""); got != "" {
		t.Errorf("no recipe present, want \"\", got %q", got)
	}

	df := filepath.Join(home, "Dockerfile")
	if err := os.WriteFile(df, []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := customDockerfile(""); got != df {
		t.Errorf("Dockerfile present, want %q, got %q", df, got)
	}

	// Containerfile takes precedence over Dockerfile.
	cf := filepath.Join(home, "Containerfile")
	if err := os.WriteFile(cf, []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := customDockerfile(""); got != cf {
		t.Errorf("Containerfile should win, want %q, got %q", cf, got)
	}
}

// A repo's own .sindri/ recipe takes precedence over the global StateDir one, so a
// project can carry its own toolchain.
func TestCustomDockerfileRepoPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SINDRI_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "Containerfile"), []byte("FROM global\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repo := t.TempDir()
	repoCf := filepath.Join(repo, ".sindri", "Containerfile")
	if err := os.MkdirAll(filepath.Dir(repoCf), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repoCf, []byte("FROM repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := customDockerfile(repo); got != repoCf {
		t.Errorf("repo .sindri/Containerfile should win over global, want %q, got %q", repoCf, got)
	}
	// With no repo recipe, it falls back to the global one.
	if got := customDockerfile(t.TempDir()); got != filepath.Join(home, "Containerfile") {
		t.Errorf("want global fallback, got %q", got)
	}
}

// imageRef is the shared default for the embedded recipe and a stable, content-
// derived tag for a custom one (identical recipes share; different ones don't).
func TestImageRef(t *testing.T) {
	if got := imageRef(nil); got != ImageName {
		t.Errorf("no custom recipe: want %q, got %q", ImageName, got)
	}
	a1 := imageRef([]byte("FROM alpine\n"))
	a2 := imageRef([]byte("FROM alpine\n"))
	b := imageRef([]byte("FROM ubuntu\n"))
	if a1 != a2 {
		t.Errorf("identical recipes must share a tag: %q vs %q", a1, a2)
	}
	if a1 == b || a1 == ImageName {
		t.Errorf("distinct/custom recipes must get distinct non-default tags: %q, %q, %q", a1, b, ImageName)
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
