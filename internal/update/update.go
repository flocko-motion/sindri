// package: update / update
// type:    logic (self-update check)
// job:     a once-a-day, best-effort check of the GitHub latest release; if it's
//          newer, print a one-line notice and drop a `sindri-do-upgrade` script,
//          which is generated because a running binary can't overwrite itself.
// limits:  best-effort and silent on failure — a check must never get in the way.
//          The network call is bounded to 2s; past that we forget it.
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

	"github.com/flo-at/sindri/internal/tools/debug"
)

// repo is the GitHub repository whose releases are checked and installed.
const repo = "flocko-motion/sindri"

// netTimeout bounds the daily background ping; explicitTimeout is longer because `sindri upgrade`
// is a deliberate request with the user waiting.
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

// MaybeNotify checks at most once a day for a newer release and, if there is one, writes the
// `sindri-do-upgrade` script and prints a one-line notice to w. Best-effort: every failure is
// ignored, and a dev build does nothing. Pass only an interactive stream.
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

// Upgrade backs `sindri upgrade [version]`: no throttle, longer timeout, since the user waits. No
// target writes the helper if the latest is newer; a target resolves that release exactly (so a
// reinstall or downgrade works). It errors only when the check couldn't run.
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

// upgradeTo resolves an explicit tag and writes the helper for it, with no newer-than check, so a
// reinstall or downgrade works. The target must be semver (v-prefix optional), rejected before any
// network call.
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

// fetchRelease resolves a release's canonical tag, bounded by timeout ("" = latest). An explicit
// tag that 404s is retried once with the "v" prefix toggled, so "0.12.1" finds "v0.12.1".
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

// errNotFound is a 404 from the API, kept distinct from transport or rate-limit failures so
// callers can drive the v-prefix fallback.
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

// githubJSON fetches a GitHub API path, preferring the gh CLI — its auth escapes the 60/hour
// anonymous limit that 403s on shared networks. Without gh, a direct GET with a User-Agent and any
// GITHUB_TOKEN/GH_TOKEN. A 404 comes back as errNotFound.
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

// httpGitHubJSON GETs api.github.com directly with a User-Agent and any env token, reporting the
// status and rate-limit headers under --debug so a 403 explains itself.
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

// newer reports whether latest is a higher semver than current. A tag that doesn't parse as vX.Y.Z
// is never newer — no nagging on versions we can't compare.
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

// parseSemver pulls major/minor/patch out of a "vX.Y.Z" tag, dropping any -pre/+meta suffix; ok is
// false unless it is three dotted integers.
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

// writeUpdater drops `sindri-do-upgrade` into ~/.local/bin and returns its path. Generated rather
// than shipped so it can replace the running binary; `sindri upgrade` only recommends it.
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

// updaterScript generates the upgrade script, OS/arch baked in so it needs no detection. One
// tarball path for every OS: ~/.local/bin only, so no second copy can shadow this one.
func updaterScript(goos, goarch, tag string) string {
	return tarballUpdaterScript(goos, goarch, tag)
}

// tarballUpdaterScript installs the goos/arch tarball over the binaries beside the running sindri,
// by rename within that dir so a live hub is swapped atomically.
func tarballUpdaterScript(goos, arch, tag string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
# Generated by sindri. Installs the %[4]s/%[2]s tarball over the current install in
# ~/.local/bin, so it needs no elevated privileges. Prefers gh (the user's auth, no
# anonymous rate limit); falls back to curl of the release API when gh is absent.
set -euo pipefail
repo=%[1]q
arch=%[2]q
tag=%[3]q   # empty = latest
goos=%[4]q
pattern="*_${goos}_${arch}.tar.gz"
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
	  | grep -o '"browser_download_url"[[:space:]]*:[[:space:]]*"[^"]*_'"${goos}_${arch}"'\.tar\.gz"' | head -1 | cut -d'"' -f4)
	[ -n "$url" ] || { echo "no ${goos}_$arch tarball in the $rel release" >&2; exit 1; }
	echo "downloading $url"
	tarball="$tmp/sindri.tar.gz"
	curl -fsSL "$url" -o "$tarball"
fi
[ -n "${tarball:-}" ] && [ -f "$tarball" ] || { echo "no ${goos}_$arch tarball downloaded" >&2; exit 1; }
# ~/.local/bin is the one install location, so upgrade in place wherever sindri is.
dest="$(dirname "$(command -v sindri 2>/dev/null || true)")"
[ -n "$dest" ] && [ -d "$dest" ] || dest="$HOME/.local/bin"
mkdir -p "$dest"
tar -C "$tmp" -xzf "$tarball"
src="$(find "$tmp" -maxdepth 1 -type d -name "sindri_*_${goos}_*" | head -1)"
[ -n "$src" ] || { echo "unexpected tarball layout" >&2; exit 1; }
# brokkr-linux is the pod's brokkr — the hub publishes it to pod-bin, so it must land too.
for bin in sindri sindri-worker brokkr brokkr-linux td yq; do
	[ -f "$src/$bin" ] || continue
	xattr -d com.apple.quarantine "$src/$bin" 2>/dev/null || true # Gatekeeper; no-op off macOS
	chmod +x "$src/$bin"
	cp "$src/$bin" "$dest/.$bin.new"
	mv -f "$dest/.$bin.new" "$dest/$bin" # atomic within $dest; safe while running
done
echo "installed to $dest — run 'sindri --version' to confirm."
`, repo, arch, tag, goos)
}
