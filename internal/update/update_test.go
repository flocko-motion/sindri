package update

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v1.2.4", "v1.2.3", true},
		{"v1.3.0", "v1.2.9", true},
		{"v2.0.0", "v1.9.9", true},
		{"v1.2.3", "v1.2.3", false},
		{"v1.2.2", "v1.2.3", false},
		{"1.2.4", "1.2.3", true},      // no 'v' prefix
		{"v1.0.0-rc1", "v0.9.0", true}, // prerelease suffix dropped
		{"dev", "v1.0.0", false},       // unparseable latest → never newer
		{"v1.0.0", "dev", false},       // unparseable current → never newer
		{"v1.2", "v1.0.0", false},      // not three parts
	}
	for _, c := range cases {
		if got := newer(c.latest, c.current); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestParseSemver(t *testing.T) {
	if a, b, c, ok := parseSemver("v1.2.3"); !ok || a != 1 || b != 2 || c != 3 {
		t.Errorf("v1.2.3 → %d.%d.%d ok=%v", a, b, c, ok)
	}
	if _, _, _, ok := parseSemver("dev"); ok {
		t.Error("dev should not parse")
	}
	if _, _, _, ok := parseSemver("1.2.3.4"); ok {
		t.Error("four parts should not parse")
	}
}

func TestCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	if c := readCache(path); c.LastCheck != "" || c.Latest != "" {
		t.Fatalf("missing cache should be zero, got %+v", c)
	}
	if err := writeCache(path, cache{LastCheck: "2026-06-25", Latest: "v1.2.3"}); err != nil {
		t.Fatal(err)
	}
	if c := readCache(path); c.LastCheck != "2026-06-25" || c.Latest != "v1.2.3" {
		t.Fatalf("round-trip lost data: %+v", c)
	}
}

func TestUpdaterScript(t *testing.T) {
	// Linux, no tag: installs the .deb via dpkg from the latest release.
	linux := updaterScript("linux", "amd64", "")
	for _, want := range []string{"#!/usr/bin/env bash", repo, "dpkg -i", `rel="latest"`} {
		if !strings.Contains(linux, want) {
			t.Errorf("linux updater script missing %q", want)
		}
	}
	if strings.Contains(linux, "tar") {
		t.Error("linux updater must not use the tarball path")
	}

	// macOS, no tag: installs the darwin tarball for the baked arch, not a .deb.
	mac := updaterScript("darwin", "arm64", "")
	for _, want := range []string{"#!/usr/bin/env bash", repo, "_darwin_", "arch=\"arm64\"", "tar -C", "com.apple.quarantine"} {
		if !strings.Contains(mac, want) {
			t.Errorf("darwin updater script missing %q", want)
		}
	}
	if strings.Contains(mac, "dpkg") {
		t.Error("darwin updater must not use dpkg")
	}

	// A pinned tag targets that exact release (tags/<tag>), on both platforms.
	for _, s := range []string{updaterScript("linux", "amd64", "v0.12.0"), updaterScript("darwin", "arm64", "v0.12.0")} {
		if !strings.Contains(s, `rel="tags/v0.12.0"`) {
			t.Errorf("pinned updater script should target tags/v0.12.0:\n%s", s)
		}
	}
}

func TestTogglePrefix(t *testing.T) {
	if got := togglePrefix("0.12.0"); got != "v0.12.0" {
		t.Errorf("togglePrefix(0.12.0) = %q, want v0.12.0", got)
	}
	if got := togglePrefix("v0.12.0"); got != "0.12.0" {
		t.Errorf("togglePrefix(v0.12.0) = %q, want 0.12.0", got)
	}
}
