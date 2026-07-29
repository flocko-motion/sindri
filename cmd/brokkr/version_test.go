package main

import (
	"runtime"
	"strings"
	"testing"
)

// TestVersionDetailReportsBuild: `brokkr version` exists to answer "which build is this,
// exactly?", so it must carry the version, the toolchain, and the platform — the last
// because brokkr is cross-built for the agent pods, and a linux/amd64 binary on a macOS
// host is a real thing to catch.
func TestVersionDetailReportsBuild(t *testing.T) {
	orig := version
	version = "1.2.3-test"
	t.Cleanup(func() { version = orig })

	got := versionDetail()
	for _, want := range []string{
		"brokkr 1.2.3-test",
		runtime.Version(),
		runtime.GOOS + "/" + runtime.GOARCH,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("version detail should carry %q:\n%s", want, got)
		}
	}
}

// TestVersionDetailNamesAMissingStamp: build time comes from an -ldflags stamp the
// Makefile applies, because Go's build info records the commit time and never the build's
// own. An unstamped build must SAY so — a silently absent line reads as a field nobody
// asked for, which is how you end up trusting a binary you did not just build.
func TestVersionDetailNamesAMissingStamp(t *testing.T) {
	orig := buildTime
	buildTime = ""
	t.Cleanup(func() { buildTime = orig })

	if got := versionDetail(); !strings.Contains(got, "not stamped") {
		t.Errorf("an unstamped build must say so, not omit the line:\n%s", got)
	}

	buildTime = "2026-07-29T07:54:39Z"
	got := versionDetail()
	if !strings.Contains(got, "2026-07-29T07:54:39Z") {
		t.Errorf("a stamped build time must be reported:\n%s", got)
	}
	if strings.Contains(got, "not stamped") {
		t.Errorf("a stamped build must not claim otherwise:\n%s", got)
	}
}

// TestVersionLineStaysOneLine: --version and the bare command's header share this string,
// and it heads the help output — provenance belongs in `brokkr version`, not above the
// list of what brokkr can do. It also must agree with the detail view on the version.
func TestVersionLineStaysOneLine(t *testing.T) {
	orig := version
	version = "9.9.9-test"
	t.Cleanup(func() { version = orig })

	line := versionLine()
	if strings.Contains(line, "\n") {
		t.Errorf("versionLine must stay a single line, got:\n%s", line)
	}
	if !strings.Contains(line, "9.9.9-test") || !strings.Contains(versionDetail(), "9.9.9-test") {
		t.Errorf("the one-line and detailed views must report the same version: %q / %q", line, versionDetail())
	}
}
