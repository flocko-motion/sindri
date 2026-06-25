// package: update / update
// type:    logic (self-update check)
// job:     a once-a-day, best-effort check of the GitHub latest release; if it's
//          newer than the running binary, print a one-line notice and drop a
//          `sindri-update` script. The script is generated (not shipped in the
//          .deb) because the running binary can't overwrite itself — the script
//          does the dpkg install in a separate process.
// limits:  best-effort and silent on failure — an update check must never get in
//          sindri's way; the actual upgrade is the generated script's job. The
//          network call is bounded to 2s (the "ping"); past that we forget it.
package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// repo is the GitHub repository whose releases are checked and installed.
const repo = "flocko-motion/sindri"

// netTimeout bounds the whole release check — the "ping": no answer in this long
// and we forget about the upgrade.
const netTimeout = 2 * time.Second

// cache is the throttle record under the user cache dir: the day we last hit the
// network and the latest release tag we saw then.
type cache struct {
	LastCheck string `json:"last_check"` // YYYY-MM-DD (UTC)
	Latest    string `json:"latest"`     // latest release tag seen
}

// MaybeNotify checks (at most once a day) whether a newer sindri release exists
// and, if so, writes the `sindri-update` script and prints a one-line notice to
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
		if latest, err := fetchLatest(); err == nil {
			c.Latest = latest
		}
		_ = os.MkdirAll(dir, 0o755)
		_ = writeCache(path, c)
	}

	if !newer(c.Latest, current) {
		return
	}
	hint := ""
	if p, err := writeUpdater(); err == nil {
		hint = updaterHint(p)
	}
	fmt.Fprintf(w, "\nsindri %s is available (you have %s) — run `sindri-update` to upgrade%s.\n\n",
		c.Latest, current, hint)
}

// fetchLatest asks GitHub for the latest release tag, bounded by netTimeout.
func fetchLatest() (string, error) {
	cl := &http.Client{Timeout: netTimeout}
	resp, err := cl.Get("https://api.github.com/repos/" + repo + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github: %s", resp.Status)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	return rel.TagName, nil
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

// writeUpdater drops the `sindri-update` script into ~/.local/bin (created if
// needed) and returns its path. It's generated rather than shipped in the .deb so
// it can install a new .deb that overwrites the running sindri binary.
func writeUpdater() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "sindri-update")
	if err := os.WriteFile(p, []byte(updaterScript()), 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// updaterHint returns "" when the updater is on PATH (so bare `sindri-update`
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

// updaterScript is the generated upgrade script: download the latest release's
// .deb and install it (needs sudo). dpkg replacing /usr/bin/sindri while sindri
// runs is safe — the running process keeps its open inode.
func updaterScript() string {
	return fmt.Sprintf(`#!/usr/bin/env bash
# Generated by sindri when a newer release was found. Installs the latest .deb.
set -euo pipefail
repo=%q
echo "fetching the latest sindri release…"
url=$(curl -fsSL "https://api.github.com/repos/$repo/releases/latest" \
  | grep -o '"browser_download_url"[[:space:]]*:[[:space:]]*"[^"]*\.deb"' | head -1 | cut -d'"' -f4)
[ -n "$url" ] || { echo "no .deb asset in the latest release" >&2; exit 1; }
tmp="$(mktemp --suffix=.deb)"
trap 'rm -f "$tmp"' EXIT
echo "downloading $url"
curl -fsSL "$url" -o "$tmp"
echo "installing (sudo dpkg -i)…"
sudo dpkg -i "$tmp" || sudo apt-get install -f -y
echo "done — run 'sindri --version' to confirm."
`, repo)
}
