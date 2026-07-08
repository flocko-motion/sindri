// package: update / update
// type:    logic (self-update check)
// job:     a once-a-day, best-effort check of the GitHub latest release; if it's
//          newer, print a one-line notice and drop a `sindri-do-upgrade` script.
//          The script is generated (not in the .deb) because the running binary
//          can't overwrite itself — it does the dpkg install in a separate run.
// limits:  best-effort and silent on failure — an update check must never get in
//          sindri's way; the actual upgrade is the generated script's job. The
//          network call is bounded to 2s (the "ping"); past that we forget it.
package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/debug"
)

// repo is the GitHub repository whose releases are checked and installed.
const repo = "flocko-motion/sindri"

// netTimeout bounds the daily background check — the "ping": no answer in this
// long and we forget about it. explicitTimeout is longer because `sindri upgrade`
// is a deliberate, blocking request — the user is waiting for an answer.
const (
	netTimeout      = 2 * time.Second
	explicitTimeout = 10 * time.Second
)

// cache is the throttle record under the user cache dir: the day we last hit the
// network and the latest release tag we saw then.
type cache struct {
	LastCheck string `json:"last_check"` // YYYY-MM-DD (UTC)
	Latest    string `json:"latest"`     // latest release tag seen
}

// MaybeNotify checks (at most once a day) whether a newer sindri release exists
// and, if so, writes the `sindri-do-upgrade` script and prints a one-line notice to
// w. It is best-effort: any failure (offline, timeout, parse, fs) is ignored, and
// it does nothing for a dev build (version "dev"/empty). The caller should only
// pass an interactive stream (e.g. a terminal stderr).
func MaybeNotify(current string, w io.Writer) {
	if current == "" || current == "dev" {
		return // unversioned local build — nothing to compare against
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "sindri")
	path := filepath.Join(dir, "update.json")

	c := readCache(path)
	today := time.Now().UTC().Format("2006-01-02")
	if c.LastCheck != today {
		// Hit the network at most once per day; record the day either way so a
		// flaky network can't make us retry (and stall 2s) every invocation.
		c.LastCheck = today
		if latest, err := fetchRelease("", netTimeout); err == nil {
			c.Latest = latest
		}
		_ = os.MkdirAll(dir, 0o755)
		_ = writeCache(path, c)
	}

	if !newer(c.Latest, current) {
		return
	}
	hint := ""
	if p, err := writeUpdater(""); err == nil { // "" = latest, the daily notice's target
		hint = updaterHint(p)
	}
	fmt.Fprintf(w, "\nsindri %s is available (you have %s) — run `sindri-do-upgrade` to upgrade%s.\n\n",
		c.Latest, current, hint)
}

// Upgrade is the explicit, on-demand action behind `sindri upgrade [version]`: it
// hits GitHub (no daily throttle) with a longer timeout since the user is waiting.
// With no target it checks the latest release and, if newer, writes the
// sindri-do-upgrade helper (targeting latest) and recommends it; if up to date it
// says so. With an explicit target it resolves that exact release (allowing a
// reinstall or a downgrade) and writes the helper to install THAT version. Returns
// an error only when the check itself couldn't run (offline, unknown version, etc.).
func Upgrade(current, target string, w io.Writer) error {
	if current == "" || current == "dev" {
		fmt.Fprintln(w, "this is a dev build (no baked version) — nothing to compare against a release.")
		return nil
	}
	if target != "" {
		return upgradeTo(current, target, w)
	}
	latest, err := fetchRelease("", explicitTimeout)
	if err != nil {
		return fmt.Errorf("couldn't check for updates: %w", err)
	}
	// Keep the daily-check cache in sync so the background notice agrees.
	if dir, e := os.UserCacheDir(); e == nil {
		d := filepath.Join(dir, "sindri")
		_ = os.MkdirAll(d, 0o755)
		_ = writeCache(filepath.Join(d, "update.json"), cache{LastCheck: time.Now().UTC().Format("2006-01-02"), Latest: latest})
	}
	if !newer(latest, current) {
		fmt.Fprintf(w, "sindri %s is up to date (latest release is %s). Pin another with `sindri upgrade <version>` (see `--list`).\n", current, latest)
		return nil
	}
	hint := ""
	if p, e := writeUpdater(""); e == nil {
		hint = updaterHint(p)
	}
	fmt.Fprintf(w, "sindri %s is available (you have %s) — run `sindri-do-upgrade` to upgrade%s.\n", latest, current, hint)
	return nil
}

// upgradeTo resolves an explicit release tag and writes the helper to install it —
// no newer-than check, so it supports reinstalling the current version or moving to
// an older one (downgrades are allowed). The target must be a semver (optionally
// v-prefixed), since releases are semver tags — anything else is rejected up front,
// before any network call.
func upgradeTo(current, target string, w io.Writer) error {
	if _, _, _, ok := parseSemver(target); !ok {
		return fmt.Errorf("invalid version %q — use a semver like v1.2.3 or 1.2.3 (see `sindri upgrade --list`)", target)
	}
	tag, err := fetchRelease(target, explicitTimeout)
	if err != nil {
		return fmt.Errorf("couldn't find release %q: %w", target, err)
	}
	hint := ""
	if p, e := writeUpdater(tag); e == nil {
		hint = updaterHint(p)
	}
	verb := "install"
	switch {
	case tag == current:
		verb = "reinstall"
	case newer(tag, current):
		verb = "upgrade to"
	case newer(current, tag):
		verb = "downgrade to"
	}
	fmt.Fprintf(w, "sindri %s selected (you have %s) — run `sindri-do-upgrade` to %s it%s.\n", tag, current, verb, hint)
	return nil
}

// ListReleases prints the repo's published release tags, newest first — so a user
// can pick one for `sindri upgrade <version>`.
func ListReleases(w io.Writer) error {
	tags, err := fetchReleaseTags(explicitTimeout)
	if err != nil {
		return fmt.Errorf("couldn't list releases: %w", err)
	}
	if len(tags) == 0 {
		fmt.Fprintf(w, "no published releases found for %s.\n", repo)
		return nil
	}
	for _, t := range tags {
		fmt.Fprintln(w, t)
	}
	return nil
}

// fetchRelease resolves a release's canonical tag from GitHub, bounded by timeout.
// An empty tag means the latest release. For an explicit tag that 404s, it retries
// once with the "v" prefix toggled (so "0.12.1" finds a "v0.12.1" tag and vice
// versa) before giving up.
func fetchRelease(tag string, timeout time.Duration) (string, error) {
	name, err := fetchReleaseExact(tag, timeout)
	if err != nil && tag != "" {
		if alt := togglePrefix(tag); alt != tag {
			if n, e := fetchReleaseExact(alt, timeout); e == nil {
				return n, nil
			}
		}
	}
	return name, err
}

// errNotFound is a release/tag the API reports as 404 — distinct from a transport or
// rate-limit failure, so callers can drive the v-prefix fallback / "no such tag".
var errNotFound = errors.New("not found")

// fetchReleaseExact fetches one release by tag ("" = latest) without any fallback.
func fetchReleaseExact(tag string, timeout time.Duration) (string, error) {
	path := "repos/" + repo + "/releases/latest"
	if tag != "" {
		path = "repos/" + repo + "/releases/tags/" + tag
	}
	body, err := githubJSON(path, timeout)
	if err != nil {
		if errors.Is(err, errNotFound) {
			if tag != "" {
				return "", fmt.Errorf("no release tagged %q", tag)
			}
			return "", fmt.Errorf("no published release found for %s — or the repo is private (an unauthenticated check 404s private repos; `gh auth login` or set GITHUB_TOKEN)", repo)
		}
		return "", err
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", err
	}
	return rel.TagName, nil
}

// githubJSON fetches a GitHub API path (e.g. "repos/o/r/releases/latest"), preferring
// the gh CLI — which uses the user's auth and so isn't subject to the low (60/hour
// per-IP) anonymous rate limit that 403s on shared networks/VPNs. Without gh it falls
// back to a direct HTTPS GET carrying a User-Agent (GitHub requires one) and any
// GITHUB_TOKEN/GH_TOKEN from the environment. A 404 comes back as errNotFound.
func githubJSON(path string, timeout time.Duration) ([]byte, error) {
	if _, err := exec.LookPath("gh"); err == nil {
		return ghAPI(path, timeout)
	}
	debug.Logf("gh not found on PATH — falling back to a direct GitHub HTTP call")
	return httpGitHubJSON(path, timeout)
}

// ghAPI runs `gh api <path>` (auth handled by gh) and returns its JSON body.
func ghAPI(path string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	debug.Logf("gh api %s", path)
	cmd := exec.CommandContext(ctx, "gh", "api", path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		debug.Logf("gh api %s failed: %v — stderr: %s", path, err, msg)
		if strings.Contains(msg, "404") || strings.Contains(strings.ToLower(msg), "not found") {
			return nil, errNotFound
		}
		if msg != "" {
			return nil, fmt.Errorf("gh api: %s", msg)
		}
		return nil, fmt.Errorf("gh api %s: %w", path, err)
	}
	return stdout.Bytes(), nil
}

// httpGitHubJSON GETs https://api.github.com/<path> directly, with a User-Agent and
// any token from the environment; it reports the status (and rate-limit headers)
// under --debug so a 403 is self-explanatory.
func httpGitHubJSON(path string, timeout time.Duration) ([]byte, error) {
	url := "https://api.github.com/" + path
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sindri")
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := githubToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
		debug.Logf("using a token from the environment for the GitHub call")
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	debug.Logf("GET %s -> %s (RateLimit-Remaining=%s, Reset=%s)", url, resp.Status,
		resp.Header.Get("X-RateLimit-Remaining"), resp.Header.Get("X-RateLimit-Reset"))
	if resp.StatusCode == http.StatusNotFound {
		return nil, errNotFound
	}
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return nil, fmt.Errorf("github: rate limit exceeded (anonymous is 60/hour per IP) — `gh auth login` or set GITHUB_TOKEN to raise it")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// githubToken returns a GitHub token from the environment (GITHUB_TOKEN or GH_TOKEN),
// or "" if none — used only by the direct-HTTP fallback (gh manages its own auth).
func githubToken() string {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// togglePrefix flips a leading "v" on a version string, so a user's "0.12.1" and the
// repo's "v0.12.1" (or vice versa) resolve to the same release.
func togglePrefix(tag string) string {
	if strings.HasPrefix(tag, "v") {
		return strings.TrimPrefix(tag, "v")
	}
	return "v" + tag
}

// fetchReleaseTags lists the repo's release tags, newest first (GitHub returns
// releases in reverse-chronological order).
func fetchReleaseTags(timeout time.Duration) ([]string, error) {
	body, err := githubJSON("repos/"+repo+"/releases?per_page=100", timeout)
	if err != nil {
		return nil, err
	}
	var rels []struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &rels); err != nil {
		return nil, err
	}
	tags := make([]string, 0, len(rels))
	for _, r := range rels {
		tags = append(tags, r.TagName)
	}
	return tags, nil
}

// newer reports whether release tag latest is a higher semver than current. A tag
// that doesn't parse as vX.Y.Z (a dev/describe build, a pre-release) is never
// considered newer — we don't nag on versions we can't compare.
func newer(latest, current string) bool {
	la, lb, lc, ok1 := parseSemver(latest)
	ca, cb, cc, ok2 := parseSemver(current)
	if !ok1 || !ok2 {
		return false
	}
	switch {
	case la != ca:
		return la > ca
	case lb != cb:
		return lb > cb
	default:
		return lc > cc
	}
}

// parseSemver pulls major/minor/patch out of a "vX.Y.Z" tag (any pre-release or
// build-metadata suffix after a "-" or "+" is dropped); ok is false unless it is
// three dotted integers.
func parseSemver(s string) (maj, min, pat int, ok bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	var err error
	if maj, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, 0, false
	}
	if min, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, 0, false
	}
	if pat, err = strconv.Atoi(parts[2]); err != nil {
		return 0, 0, 0, false
	}
	return maj, min, pat, true
}

func readCache(path string) cache {
	var c cache
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	return c
}

func writeCache(path string, c cache) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// writeUpdater drops the `sindri-do-upgrade` script into ~/.local/bin (created if
// needed) and returns its path. It's generated rather than shipped in the .deb so
// it can install a new .deb that overwrites the running sindri binary. (Named
// distinctly from the `sindri upgrade` command, which only checks and recommends
// running this.)
func writeUpdater(tag string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "sindri-do-upgrade")
	if err := os.WriteFile(p, []byte(updaterScript(runtime.GOOS, runtime.GOARCH, tag)), 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// updaterHint returns "" when the updater is on PATH (so bare `sindri-do-upgrade`
// runs), else its full path in parentheses for the notice.
func updaterHint(path string) string {
	dir := filepath.Dir(path)
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if d == dir {
			return ""
		}
	}
	return " (" + path + ")"
}

// updaterScript is the generated upgrade script for the host OS. The OS/arch are
// baked in at generation time (this runs on the same machine that generated it),
// so the script picks the right release asset without runtime detection. Replacing
// the binaries while sindri/the hub runs is safe — the running process keeps its
// open inode (dpkg and the atomic mv below both preserve it).
func updaterScript(goos, goarch, tag string) string {
	if goos == "darwin" {
		return darwinUpdaterScript(goarch, tag)
	}
	return debUpdaterScript(tag)
}

// debUpdaterScript downloads a release's .deb and installs it (needs sudo). An empty
// tag installs the latest; a set tag installs exactly that release. It prefers `gh
// release download` (the user's auth, no anonymous rate limit) and falls back to a
// direct curl of the release API when gh isn't installed.
func debUpdaterScript(tag string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
# Generated by sindri. Installs the sindri .deb (gh preferred, curl fallback).
set -euo pipefail
repo=%[1]q
tag=%[2]q   # empty = latest
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
if command -v gh >/dev/null 2>&1; then
	echo "fetching via gh (${tag:-latest})…"
	args=(--repo "$repo" --pattern '*.deb' --dir "$tmp" --clobber)
	[ -n "$tag" ] && args=("$tag" "${args[@]}")
	gh release download "${args[@]}"
	deb="$(find "$tmp" -maxdepth 1 -name '*.deb' | head -1)"
else
	rel="latest"; [ -n "$tag" ] && rel="tags/$tag"
	echo "fetching via curl ($rel)…"
	url=$(curl -fsSL "https://api.github.com/repos/$repo/releases/$rel" \
	  | grep -o '"browser_download_url"[[:space:]]*:[[:space:]]*"[^"]*\.deb"' | head -1 | cut -d'"' -f4)
	[ -n "$url" ] || { echo "no .deb asset in the $rel release" >&2; exit 1; }
	echo "downloading $url"
	deb="$tmp/sindri.deb"
	curl -fsSL "$url" -o "$deb"
fi
[ -n "${deb:-}" ] && [ -f "$deb" ] || { echo "no .deb downloaded" >&2; exit 1; }
echo "installing (sudo dpkg -i)…"
sudo dpkg -i "$deb" || sudo apt-get install -f -y
echo "done — run 'sindri --version' to confirm."
`, repo, tag)
}

// darwinUpdaterScript downloads the latest macOS tarball for arch and installs it
// over the current binaries. It replaces them next to the running sindri (so it
// upgrades whichever install is in use), via a rename within that dir so a live
// sindri/hub is swapped atomically, and clears the Gatekeeper quarantine.
func darwinUpdaterScript(arch, tag string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
# Generated by sindri. Installs the macOS tarball (darwin/%[2]s) over the current
# install — no sudo when it lives in ~/.local/bin. Prefers gh (the user's auth, no
# anonymous rate limit); falls back to curl of the release API when gh is absent.
set -euo pipefail
repo=%[1]q
arch=%[2]q
tag=%[3]q   # empty = latest
pattern="*_darwin_${arch}.tar.gz"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
if command -v gh >/dev/null 2>&1; then
	echo "fetching via gh (${tag:-latest})…"
	args=(--repo "$repo" --pattern "$pattern" --dir "$tmp" --clobber)
	[ -n "$tag" ] && args=("$tag" "${args[@]}")
	gh release download "${args[@]}"
	tarball="$(find "$tmp" -maxdepth 1 -name "$pattern" | head -1)"
else
	rel="latest"; [ -n "$tag" ] && rel="tags/$tag"
	echo "fetching via curl ($rel)…"
	url=$(curl -fsSL "https://api.github.com/repos/$repo/releases/$rel" \
	  | grep -o '"browser_download_url"[[:space:]]*:[[:space:]]*"[^"]*_darwin_'"$arch"'\.tar\.gz"' | head -1 | cut -d'"' -f4)
	[ -n "$url" ] || { echo "no darwin_$arch tarball in the $rel release" >&2; exit 1; }
	echo "downloading $url"
	tarball="$tmp/sindri.tar.gz"
	curl -fsSL "$url" -o "$tarball"
fi
[ -n "${tarball:-}" ] && [ -f "$tarball" ] || { echo "no darwin_$arch tarball downloaded" >&2; exit 1; }
# Upgrade wherever the current sindri lives; fall back to ~/.local/bin.
dest="$(dirname "$(command -v sindri 2>/dev/null || true)")"
[ -n "$dest" ] && [ -d "$dest" ] || dest="$HOME/.local/bin"
mkdir -p "$dest"
tar -C "$tmp" -xzf "$tarball"
src="$(find "$tmp" -maxdepth 1 -type d -name 'sindri_*_darwin_*' | head -1)"
[ -n "$src" ] || { echo "unexpected tarball layout" >&2; exit 1; }
for bin in sindri sindri-worker brokkr td yq; do
	[ -f "$src/$bin" ] || continue
	xattr -d com.apple.quarantine "$src/$bin" 2>/dev/null || true # clear Gatekeeper
	chmod +x "$src/$bin"
	cp "$src/$bin" "$dest/.$bin.new"
	mv -f "$dest/.$bin.new" "$dest/$bin" # atomic within $dest; safe while running
done
echo "installed to $dest — run 'sindri --version' to confirm."
`, repo, arch, tag)
}
