// package: container / image
// type:    logic (backend-agnostic image-build recipe)
// job:     the agent image's identity (ImageName) and the backend-agnostic build recipe: hash
// the EMBEDDED context plus the week into a key, materialize, build — delegating
// exists?/build to an ImageBuilder the pod and apple adapters supply.
// limits:  no backend specifics (-> adapter/container/*); pod lifecycle is the hub's.
package container

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/tools/paths"
)

// imageBase is the image repository; ImageName is the default (embedded-recipe) ref.
// A custom recipe builds imageBase:custom-<hash> instead (see imageRef).
const imageBase = "sindri-agent"
const ImageName = imageBase + ":latest"

// buildContext is the image's whole build context, embedded so the binary carries its own recipe.
//
//go:embed all:buildctx
var buildContext embed.FS

// ImageBuilder is the backend-specific half of image building: presence, and build.
type ImageBuilder interface {
	// ImageExists reports whether ref is present; an error means UNKNOWN, not absent.
	ImageExists(ref string) (bool, error)
	// Build builds ref from ctxDir/dockerfile; pull re-fetches the base image. The returned map is
	// this build's tool-version manifest (-> ParseVersionManifest), nil if none was found.
	Build(ref, ctxDir, dockerfile string, pull bool, out io.Writer) (map[string]string, error)
}

// versionMarker prefixes each "tool version" line the Dockerfile prints while installing one, so the
// build log every ImageBuilder.Build already captures doubles as the version manifest — nothing baked
// into the image itself to fetch back out, no exec into a built container.
const versionMarker = "SINDRI-TOOL-VERSION "

// ParseVersionManifest pulls "tool version" pairs out of a build log, keyed by the marker lines an
// ImageBuilder.Build implementation captured (stdout+stderr of the build it just ran). The marker is
// found anywhere in the line, not just at its start: a BuildKit-driven builder (-> applecontainer)
// prefixes every RUN line with its own step/elapsed decoration, which must not hide the marker. Only
// the first space after the marker splits tool from version — the rest of the line, however many
// words, is kept whole as the version, so a CLI whose own --version isn't a bare token still compares
// correctly against the identically-unprocessed value the host side reads (-> hosttools.Versions).
func ParseVersionManifest(buildLog string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(buildLog, "\n") {
		i := strings.Index(line, versionMarker)
		if i < 0 {
			continue
		}
		rest := strings.TrimSpace(line[i+len(versionMarker):])
		tool, version, ok := strings.Cut(rest, " ")
		version = strings.TrimSpace(version)
		if !ok || tool == "" || version == "" {
			continue
		}
		m[tool] = version
	}
	return m
}

// ImageManifest is ref's tool-version manifest as of its last successful build (nil, nil if it has
// never been built here, or nothing was recorded) — a local cache read, never a build or a container
// call, so a caller may check it any time without forcing an image to exist first.
func ImageManifest(ref string) (map[string]string, error) {
	cacheDir, err := buildCacheDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(manifestFile(cacheDir, ref))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse cached manifest for %s: %w", ref, err)
	}
	return m, nil
}

func manifestFile(cacheDir, ref string) string {
	return filepath.Join(cacheDir, "manifest-"+tagOf(ref)+".json")
}

// buildProgress collapses a build's output into one in-place status line; finish() ends it.
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

// EnsureImageWith runs the build recipe and returns the image reference to run. A custom recipe
// gets a content-derived tag, so repos with different recipes don't clobber each other's.
func EnsureImageWith(projectRoot, containerfile string, out io.Writer, b ImageBuilder) (string, error) {
	return buildImage(projectRoot, containerfile, out, b, false)
}

// RebuildImageWith forces a rebuild and re-pulls the base — the way to pick up a newer base
// (a new Go in golang:latest) that both caches would otherwise keep stale.
func RebuildImageWith(projectRoot, containerfile string, out io.Writer, b ImageBuilder) (string, error) {
	return buildImage(projectRoot, containerfile, out, b, true)
}

// buildImage is the shared recipe; force skips the up-to-date short-circuit and
// re-pulls the base (see EnsureImageWith / RebuildImageWith).
func buildImage(projectRoot, containerfile string, out io.Writer, b ImageBuilder, force bool) (string, error) {
	// Precedence: an explicit config `containerfile` (resolved by the caller), then a
	// repo-local .sindri/ recipe, then the global StateDir recipe, then the embedded
	// default (read once, folded into the build key so edits rebuild).
	custom := containerfile
	if custom == "" {
		custom = customDockerfile(projectRoot)
	}
	var customData []byte
	if custom != "" {
		data, err := os.ReadFile(custom)
		if err != nil {
			return "", fmt.Errorf("read custom image recipe %s: %w", custom, err)
		}
		customData = data
	}
	ref := imageRef(customData)

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
		return "", fmt.Errorf("hash embedded build context: %w", err)
	}
	h.Write([]byte(fmt.Sprintf("%d-%d", year, week)))
	if customData != nil {
		h.Write([]byte("custom"))
		h.Write(customData)
	}
	buildKey := fmt.Sprintf("%x", h.Sum(nil))[:16]

	cacheDir, err := buildCacheDir()
	if err != nil {
		return "", err
	}
	// Per-ref key file, so alternating between repos with different recipes doesn't
	// invalidate each other's cache and force a rebuild every switch.
	keyFile := filepath.Join(cacheDir, "build-key-"+tagOf(ref))
	if cached, err := os.ReadFile(keyFile); !force && err == nil && strings.TrimSpace(string(cached)) == buildKey {
		present, perr := b.ImageExists(ref)
		if perr != nil {
			// Presence unknown: surface it rather than silently rebuilding or skipping.
			return "", fmt.Errorf("check whether image %s already exists: %w", ref, perr)
		}
		if present {
			return ref, nil // up to date and the image is actually present
		}
	}

	// Materialize the embedded context into a writable staging dir.
	ctxDir := filepath.Join(cacheDir, "buildctx")
	if err := materialize(ctxDir); err != nil {
		return "", err
	}
	// Overlay the custom recipe onto the materialized context: the embedded
	// support files (entrypoint, shims, yazi.sh) stay put, so a custom Dockerfile
	// can COPY them and keep the agent contract.
	if customData != nil {
		if err := os.WriteFile(filepath.Join(ctxDir, "Dockerfile"), customData, 0o644); err != nil {
			return "", fmt.Errorf("apply custom image recipe: %w", err)
		}
		fmt.Fprintf(out, "Using custom image recipe: %s\n", custom)
	}

	fmt.Fprintf(out, "Building agent image %s...\n", ref)
	// Collapse the (verbose) build log into one in-place-updating line, so the caller
	// sees progress happening without pages of buildkit output.
	bp := &buildProgress{out: out}
	manifest, buildErr := b.Build(ref, ctxDir, filepath.Join(ctxDir, "Dockerfile"), force, bp)
	bp.finish()
	if buildErr != nil {
		return "", buildErr
	}
	if err := os.WriteFile(keyFile, []byte(buildKey), 0o644); err != nil {
		return "", fmt.Errorf("write build key: %w", err)
	}
	// Best-effort: nil (a custom recipe with no version lines, or podman's own layer cache skipping
	// the RUN that prints them) means nothing new to compare, not a failure — any older manifest stays.
	if len(manifest) > 0 {
		if data, err := json.Marshal(manifest); err == nil {
			_ = os.WriteFile(manifestFile(cacheDir, ref), data, 0o644)
		}
	}
	return ref, nil
}

// imageRef is the tag to build/run: the shared default for the embedded recipe, or
// a content-derived tag for a custom recipe so distinct recipes never share (or
// clobber) a tag. Identical recipes hash to the same tag and are reused.
func imageRef(customData []byte) string {
	if len(customData) == 0 {
		return ImageName
	}
	sum := sha256.Sum256(customData)
	return fmt.Sprintf("%s:custom-%x", imageBase, sum[:6])
}

// tagOf returns the tag portion of an image reference (after the last ':').
func tagOf(ref string) string {
	if i := strings.LastIndexByte(ref, ':'); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

// customDockerfile is the user's image recipe, or "" — the repo's .sindri/ first, then the
// global one. It fully replaces the embedded recipe, so it must still honour the agent contract
// (non-root `sindri` user, the sindri-agent entrypoint, WORKDIR /workspace).
func customDockerfile(projectRoot string) string {
	dirs := []string{}
	if projectRoot != "" {
		dirs = append(dirs, filepath.Join(projectRoot, ".sindri"))
	}
	dirs = append(dirs, paths.StateDir())
	for _, dir := range dirs {
		for _, name := range []string{"Containerfile", "Dockerfile"} {
			p := filepath.Join(dir, name)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				return p
			}
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
