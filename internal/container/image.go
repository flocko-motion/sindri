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
	"bytes"
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
// repo. Arch-specific tools (yq, yazi) are downloaded in-container by the
// Dockerfile for the pod's own OS/arch, not copied from the (possibly macOS) host.
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

	// Materialize the embedded context into a writable staging dir. Tools that
	// must match the pod's OS/arch (yq, yazi) are downloaded in-container by the
	// Dockerfile, not copied from the host — the host may be macOS/arm64 while the
	// pod is Linux.
	ctxDir := filepath.Join(cacheDir, "buildctx")
	if err := materialize(ctxDir); err != nil {
		return err
	}

	fmt.Fprintf(out, "Building agent image %s...\n", ImageName)
	// Capture podman's output alongside streaming it, so a failure carries the
	// actual diagnostic — not a bare "exit status 125". The hub may be running
	// detached (its stream goes to .sindri/hub.log), so the returned error is often
	// the only place the caller sees why.
	var captured bytes.Buffer
	cmd := exec.Command("podman", "build", "-t", ImageName, "-f", filepath.Join(ctxDir, "Dockerfile"), ctxDir)
	cmd.Stdout = io.MultiWriter(out, &captured)
	cmd.Stderr = io.MultiWriter(out, &captured)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("podman build failed (%v):\n%s", err, buildFailureDetail(captured.String()))
	}
	if err := os.WriteFile(keyFile, []byte(buildKey), 0o644); err != nil {
		return fmt.Errorf("write build key: %w", err)
	}
	return nil
}

// buildFailureDetail distills podman's build output for an error message: the
// meaningful tail (its own diagnostics), plus a hint for the most common cause of
// a pre-build failure — podman not being reachable, notably on macOS/Windows where
// it runs in a VM that must be started. Falls back to the hint alone when podman
// printed nothing.
func buildFailureDetail(out string) string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimRight(l, "\r"))
		}
	}
	if len(lines) > 12 { // keep the tail, where podman's error lands
		lines = lines[len(lines)-12:]
	}
	detail := strings.Join(lines, "\n")
	low := strings.ToLower(out)
	unreachable := detail == "" ||
		strings.Contains(low, "cannot connect") ||
		strings.Contains(low, "connection refused") ||
		strings.Contains(low, "no such host") ||
		strings.Contains(low, "machine") ||
		strings.Contains(low, "is the podman")
	if unreachable {
		hint := "hint: podman doesn't look reachable. On macOS/Windows it runs in a VM — run `podman machine init` (first time) then `podman machine start`; check with `podman info`."
		if detail == "" {
			return hint
		}
		detail += "\n" + hint
	}
	return detail
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

