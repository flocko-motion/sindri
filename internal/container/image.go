// package: container / image
// type:    adapter (podman)
// job:     the agent image identity (ImageName) and build. The build context
//          (Dockerfile, entrypoint, shims) is EMBEDDED in the binary, so an
//          installed sindri can build the image for ANY orchestrated repo, not
//          just the sindri repo.
// limits:  worker/reviewer container lifecycle lives in internal/hub. Ensure
//          materializes the embedded context to a cache dir, stages yq, and
//          builds via podman when that context or the weekly key is stale.
package container

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const ImageName = "sindri-agent:test"

// buildContext is the agent image's whole build context — Dockerfile, the
// entrypoint, the yazi helper, and the docker shims — embedded so the binary
// carries its own image recipe and never depends on files in the orchestrated
// repo. `yq` is NOT embedded (it's a separate licensed binary); it is staged
// from the host PATH into the materialized context at build time.
//
//go:embed all:buildctx
var buildContext embed.FS

// Ensure builds the container image if the embedded build context changed or the
// weekly cache key is stale. Build progress is written to out (so the hub can
// tee it into an agent's live-screen region during launch). It is independent of
// projectRoot — the recipe is embedded — so it works for any orchestrated repo.
func Ensure(projectRoot string, out io.Writer) error {
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
	buildKey := fmt.Sprintf("%x", h.Sum(nil))[:16]

	cacheDir, err := buildCacheDir()
	if err != nil {
		return err
	}
	keyFile := filepath.Join(cacheDir, "build-key")
	if cached, err := os.ReadFile(keyFile); err == nil &&
		strings.TrimSpace(string(cached)) == buildKey &&
		exec.Command("podman", "image", "exists", ImageName).Run() == nil {
		return nil // up to date and the image is actually present
	}

	// Materialize the embedded context into a writable staging dir and stage yq
	// (which the Dockerfile COPYs) alongside it.
	ctxDir := filepath.Join(cacheDir, "buildctx")
	if err := materialize(ctxDir); err != nil {
		return err
	}
	if err := stageYq(ctxDir); err != nil {
		return err
	}

	fmt.Fprintf(out, "Building agent image %s...\n", ImageName)
	cmd := exec.Command("podman", "build", "-t", ImageName, "-f", filepath.Join(ctxDir, "Dockerfile"), ctxDir)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("image build failed: %w", err)
	}
	if err := os.WriteFile(keyFile, []byte(buildKey), 0o644); err != nil {
		return fmt.Errorf("write build key: %w", err)
	}
	return nil
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

// stageYq copies the host's yq into the build context (the Dockerfile COPYs it).
// yq is bundled by the .deb, so it is expected on PATH; its absence is a loud
// failure rather than a silently broken image.
func stageYq(ctxDir string) error {
	path, err := exec.LookPath("yq")
	if err != nil {
		return fmt.Errorf("yq not found on PATH — it ships with sindri and the agent image needs it: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read yq: %w", err)
	}
	if err := os.WriteFile(filepath.Join(ctxDir, "yq"), data, 0o755); err != nil {
		return fmt.Errorf("stage yq: %w", err)
	}
	return nil
}
