// package: container / image
// type:    adapter (podman)
// job:     the agent image identity (ImageName) and build. The build context
//          (Dockerfile, entrypoint, shims) is EMBEDDED in the binary, so an
//          installed sindri can build the image for ANY orchestrated repo, not
//          just the sindri repo.
// limits:  worker/reviewer container lifecycle lives in internal/hub. Ensure
//          materializes the embedded context to a cache dir and builds via podman
//          when that context or the weekly key is stale.
package container

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/paths"
)

const ImageName = "sindri-agent:latest"

// buildContext is the agent image's whole build context — Dockerfile, the
// entrypoint, the yazi helper, and the docker shims — embedded so the binary
// carries its own image recipe and never depends on files in the orchestrated
// repo. Arch-specific tools (yq, yazi) are downloaded in-container by the
// Dockerfile for the pod's own OS/arch, not copied from the (possibly macOS) host.
//
//go:embed all:buildctx
var buildContext embed.FS

// Ensure builds the container image if the embedded build context changed or the
// weekly cache key is stale. Build progress is written to out (so the hub can
// tee it into an agent's live-screen region during launch). It is independent of
// projectRoot — the recipe is embedded — so it works for any orchestrated repo.
// ImageBuilder is the backend-specific slice of image building that the shared
// recipe delegates to: whether the image is already present, and how to build it.
// The podman and apple-container adapters each provide one.
type ImageBuilder interface {
	ImageExists() bool
	Build(ctxDir, dockerfile string, out io.Writer) error
}

// buildProgress collapses a build's plain, line-oriented output into a single
// in-place status line (carriage-return overwrite, padded to erase the previous
// one), so a build — cached or fresh — shows one moving line instead of scrolling
// the whole buildkit log. finish() ends the line. A non-TTY consumer just sees the
// last state each CR yields; the surrounding messages are written separately.
type buildProgress struct {
	out  io.Writer
	line []byte
}

func (p *buildProgress) Write(b []byte) (int, error) {
	for _, c := range b {
		if c == '\n' || c == '\r' {
			p.flush()
			continue
		}
		p.line = append(p.line, c)
	}
	return len(b), nil
}

func (p *buildProgress) flush() {
	s := strings.TrimSpace(string(p.line))
	p.line = p.line[:0]
	if s == "" {
		return
	}
	if r := []rune(s); len(r) > 90 {
		s = string(r[:90]) + "…"
	}
	fmt.Fprintf(p.out, "\r  %-92s", s) // CR + pad to overwrite a longer previous line
}

func (p *buildProgress) finish() {
	p.flush()
	fmt.Fprint(p.out, "\n")
}

// EnsureImageWith runs the shared build recipe — hash the embedded context + ISO
// week (+ any custom Dockerfile) into a key, skip when it's unchanged and the image
// is present, else materialize and build — delegating the backend-specific steps
// (image-exists check, build invocation) to b. It's independent of projectRoot: the
// recipe is embedded, so it works for any orchestrated repo.
func EnsureImageWith(projectRoot string, out io.Writer, b ImageBuilder) error {
	// A custom recipe in the central sindri home replaces the embedded Dockerfile
	// (read once, folded into the build key below so edits rebuild).
	custom := customDockerfile()
	var customData []byte
	if custom != "" {
		data, err := os.ReadFile(custom)
		if err != nil {
			return fmt.Errorf("read custom image recipe %s: %w", custom, err)
		}
		customData = data
	}

	// Hash the embedded context (Dockerfile + entrypoint + shims) plus the ISO
	// week, so any change to the recipe — or a new week — triggers a rebuild.
	year, week := time.Now().ISOWeek()
	h := sha256.New()
	if err := fs.WalkDir(buildContext, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, e := buildContext.ReadFile(p)
		if e != nil {
			return e
		}
		h.Write([]byte(p))
		h.Write(data)
		return nil
	}); err != nil {
		return fmt.Errorf("hash embedded build context: %w", err)
	}
	h.Write([]byte(fmt.Sprintf("%d-%d", year, week)))
	if customData != nil {
		h.Write([]byte("custom"))
		h.Write(customData)
	}
	buildKey := fmt.Sprintf("%x", h.Sum(nil))[:16]

	cacheDir, err := buildCacheDir()
	if err != nil {
		return err
	}
	keyFile := filepath.Join(cacheDir, "build-key")
	if cached, err := os.ReadFile(keyFile); err == nil &&
		strings.TrimSpace(string(cached)) == buildKey && b.ImageExists() {
		return nil // up to date and the image is actually present
	}

	// Materialize the embedded context into a writable staging dir. Tools that
	// must match the pod's OS/arch (yq, yazi) are downloaded in-container by the
	// Dockerfile, not copied from the host — the host may be macOS/arm64 while the
	// pod is Linux.
	ctxDir := filepath.Join(cacheDir, "buildctx")
	if err := materialize(ctxDir); err != nil {
		return err
	}
	// Overlay the custom recipe onto the materialized context: the embedded
	// support files (entrypoint, shims, yazi.sh) stay put, so a custom Dockerfile
	// can COPY them and keep the agent contract.
	if customData != nil {
		if err := os.WriteFile(filepath.Join(ctxDir, "Dockerfile"), customData, 0o644); err != nil {
			return fmt.Errorf("apply custom image recipe: %w", err)
		}
		fmt.Fprintf(out, "Using custom image recipe: %s\n", custom)
	}

	fmt.Fprintf(out, "Building agent image %s...\n", ImageName)
	// Collapse the (verbose) build log into one in-place-updating line, so the caller
	// sees progress happening without pages of buildkit output.
	bp := &buildProgress{out: out}
	buildErr := b.Build(ctxDir, filepath.Join(ctxDir, "Dockerfile"), bp)
	bp.finish()
	if buildErr != nil {
		return buildErr
	}
	if err := os.WriteFile(keyFile, []byte(buildKey), 0o644); err != nil {
		return fmt.Errorf("write build key: %w", err)
	}
	return nil
}

// customDockerfile returns the path to a user-provided image recipe in the central
// sindri home (paths.StateDir), or "" if none. A file named "Containerfile" or
// "Dockerfile" there fully replaces the embedded recipe — maximum customization
// (extra tools, private base images) without editing the binary. The recipe must
// still honor the agent contract: a non-root `sindri` user, /usr/local/bin/sindri
// pointing at the mounted worker, the sindri-agent entrypoint, and WORKDIR
// /workspace — easiest by starting from a copy of the embedded Dockerfile.
func customDockerfile() string {
	for _, name := range []string{"Containerfile", "Dockerfile"} {
		p := filepath.Join(paths.StateDir(), name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// buildCacheDir is the per-user cache dir for the image build (recipe staging +
// build key), independent of any orchestrated repo.
func buildCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate cache dir: %w", err)
	}
	dir := filepath.Join(base, "sindri", "image")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create image cache dir: %w", err)
	}
	return dir, nil
}

// materialize writes the embedded buildctx tree into dir (cleared first), so
// podman has a real context directory to build from. The "buildctx/" prefix is
// stripped, so the Dockerfile sits at the context root.
func materialize(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clear staging dir: %w", err)
	}
	return fs.WalkDir(buildContext, "buildctx", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("buildctx", p)
		dst := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, e := buildContext.ReadFile(p)
		if e != nil {
			return e
		}
		return os.WriteFile(dst, data, 0o755)
	})
}

