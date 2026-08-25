package container

import (
	"io"
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
		"buildctx/go-upgrade.sh",
		"buildctx/shims/docker",
		"buildctx/shims/docker-compose",
		"buildctx/shims/git",
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

// ParseVersionManifest pulls out only the marker lines, ignoring the rest of a real build log.
func TestParseVersionManifest(t *testing.T) {
	log := "Step 1/10 : FROM golang:latest\n" +
		"SINDRI-TOOL-VERSION go go1.26.4\n" +
		"some other build noise\n" +
		"SINDRI-TOOL-VERSION node v22.11.0\n" +
		"SINDRI-TOOL-VERSION openspec 1.8.0\n" +
		"Successfully tagged sindri-agent:latest\n"
	got := ParseVersionManifest(log)
	want := map[string]string{"go": "go1.26.4", "node": "v22.11.0", "openspec": "1.8.0"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

// A marker line with no version at all is skipped rather than misparsed into a bogus entry — it
// must not surface as a false mismatch.
func TestParseVersionManifestIgnoresMalformedLines(t *testing.T) {
	got := ParseVersionManifest("SINDRI-TOOL-VERSION onlyname\nSINDRI-TOOL-VERSION \n")
	if len(got) != 0 {
		t.Errorf("a marker line with no version should be ignored, got %v", got)
	}
}

// A version that isn't a bare token (a CLI whose own --version prints more than one word) must be
// kept whole, not split apart and rejected — the asymmetry that used to silently drop such an entry
// while the host side (which reads the same raw command output unprocessed) kept it.
func TestParseVersionManifestKeepsAMultiWordVersion(t *testing.T) {
	got := ParseVersionManifest("SINDRI-TOOL-VERSION openspec openspec version 1.8.0\n")
	if got["openspec"] != "openspec version 1.8.0" {
		t.Errorf("want the whole remainder kept as the version, got %v", got)
	}
}

// BuildKit-style progress output prefixes every RUN line with its own step/elapsed marker
// ("#12 0.53 ...") rather than the marker opening the line — it must still be found.
func TestParseVersionManifestSurvivesLineDecoration(t *testing.T) {
	got := ParseVersionManifest("#12 0.53 SINDRI-TOOL-VERSION go go1.26.4\n")
	if got["go"] != "go1.26.4" {
		t.Errorf("want the marker found mid-line, got %v", got)
	}
}

// podman/buildah echo a RUN instruction's own SOURCE TEXT as a "STEP N/M: RUN ..." line — on a
// layer-cache hit that echo, followed by "--> Using cache", is the ONLY line captured; the echo that
// would print a real reading never runs. If the marker appeared whole in the Dockerfile's RUN
// argument, that echo would be misread as a real (fabricated) manifest. Pinned against the
// instruction as buildctx/Dockerfile actually writes it: the marker split across two adjacent shell
// strings so it never appears whole in its own source.
func TestParseVersionManifestIgnoresACacheHitEchoOfItsOwnInstruction(t *testing.T) {
	log := "STEP 24/41: RUN m=\"SINDRI-TOOL-\"\"VERSION\"   " +
		"&& echo \"$m go $(go version | awk '{print $3}')\"   " +
		"&& echo \"$m node $(node --version)\"   " +
		"&& echo \"$m openspec $(openspec --version)\"\n" +
		"--> Using cache 9ab2\n"
	got := ParseVersionManifest(log)
	if len(got) != 0 {
		t.Errorf("a cache-hit echo of the RUN's own source must not be read as a manifest, got %v", got)
	}
}

// BuildKit (applecontainer's builder) echoes and cache-hits the same instruction its own way
// ("#12 [8/30] RUN ...", then "#12 CACHED") — the same fix protects it too, since both builders
// share one Dockerfile.
func TestParseVersionManifestIgnoresABuildKitCacheHitEchoOfItsOwnInstruction(t *testing.T) {
	log := "#12 [8/30] RUN m=\"SINDRI-TOOL-\"\"VERSION\" " +
		"&& echo \"$m go $(go version | awk '{print $3}')\" " +
		"&& echo \"$m node $(node --version)\" " +
		"&& echo \"$m openspec $(openspec --version)\"\n" +
		"#12 CACHED\n"
	got := ParseVersionManifest(log)
	if len(got) != 0 {
		t.Errorf("a BuildKit cache-hit echo of the RUN's own source must not be read as a manifest, got %v", got)
	}
}

// ImageManifest must not error just because nothing has been built here yet — that is the normal
// state for a fresh install, not a failure.
func TestImageManifestNotYetBuilt(t *testing.T) {
	setIsolatedCacheDir(t)
	got, err := ImageManifest("sindri-agent:no-such-tag")
	if err != nil {
		t.Fatalf("ImageManifest: %v", err)
	}
	if got != nil {
		t.Errorf("no build yet, want nil manifest, got %v", got)
	}
}

// fakeBuilder is a minimal ImageBuilder for exercising buildImage's manifest plumbing without a
// real container runtime.
type fakeBuilder struct {
	exists   bool
	manifest map[string]string
	builds   int
}

func (f *fakeBuilder) ImageExists(string) (bool, error) { return f.exists, nil }

func (f *fakeBuilder) Build(_, _, _ string, _ bool, _ io.Writer) (map[string]string, error) {
	f.builds++
	return f.manifest, nil
}

// setIsolatedCacheDir points buildCacheDir's os.UserCacheDir() at a fresh temp dir on every OS.
// XDG_CACHE_HOME alone isn't enough: os.UserCacheDir honours it only on Unix — on darwin it
// hardcodes $HOME/Library/Caches and ignores XDG_CACHE_HOME outright, so a test that sets only the
// latter runs against the developer's REAL cache on a Mac, clearing their real staging dir
// (materialize does os.RemoveAll) and writing a fabricated manifest a later real hub startup reads.
func setIsolatedCacheDir(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
}

// A successful build's manifest must be readable back through ImageManifest — the whole point of
// baking it in at build time rather than execing into a pod to ask.
func TestBuildImagePersistsManifestForImageManifestToRead(t *testing.T) {
	setIsolatedCacheDir(t)
	t.Setenv("SINDRI_HOME", t.TempDir())
	fb := &fakeBuilder{manifest: map[string]string{"go": "go1.26.4"}}
	ref, err := EnsureImageWith("", "", io.Discard, fb)
	if err != nil {
		t.Fatalf("EnsureImageWith: %v", err)
	}
	got, err := ImageManifest(ref)
	if err != nil {
		t.Fatalf("ImageManifest: %v", err)
	}
	if got["go"] != "go1.26.4" {
		t.Errorf("manifest not persisted: got %v", got)
	}
}

// Once an image is up to date, EnsureImageWith must not call Build again (the whole point of the
// cache) — and the manifest from the ORIGINAL build must still be readable, not lost.
func TestBuildImageSkipsRebuildButManifestSurvives(t *testing.T) {
	setIsolatedCacheDir(t)
	t.Setenv("SINDRI_HOME", t.TempDir())
	fb := &fakeBuilder{manifest: map[string]string{"go": "go1.26.4"}}
	ref, err := EnsureImageWith("", "", io.Discard, fb)
	if err != nil {
		t.Fatalf("first EnsureImageWith: %v", err)
	}
	fb.exists = true // now "already built and present" — the second call should short-circuit
	if _, err := EnsureImageWith("", "", io.Discard, fb); err != nil {
		t.Fatalf("second EnsureImageWith: %v", err)
	}
	if fb.builds != 1 {
		t.Errorf("an up-to-date image should not rebuild, Build called %d time(s)", fb.builds)
	}
	got, err := ImageManifest(ref)
	if err != nil {
		t.Fatalf("ImageManifest: %v", err)
	}
	if got["go"] != "go1.26.4" {
		t.Errorf("cached manifest should still be readable after a skipped rebuild: got %v", got)
	}
}
