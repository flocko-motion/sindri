package update

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/tools/debug"
)

// fakeGH puts a stub `gh` on PATH: `gh api …/releases/latest` returns a tag,
// `…/releases/tags/…` 404s, and `…/releases` returns a list — so the gh-preferred
// fetch path is exercised without a network or real gh.
func fakeGH(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	script := `#!/bin/sh
case "$2" in
  *releases/latest) echo '{"tag_name":"v9.9.9"}' ;;
  *releases/tags/*) echo "gh: Not Found (HTTP 404)" 1>&2; exit 1 ;;
  *releases*) echo '[{"tag_name":"v9.9.9"},{"tag_name":"v9.9.8"}]' ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestFetchReleaseViaGh(t *testing.T) {
	fakeGH(t)
	got, err := fetchReleaseExact("", time.Second)
	if err != nil || got != "v9.9.9" {
		t.Fatalf("latest via gh = %q, %v; want v9.9.9", got, err)
	}
}

func TestFetchReleaseNotFoundViaGh(t *testing.T) {
	fakeGH(t)
	_, err := fetchReleaseExact("v0.0.0", time.Second)
	if err == nil {
		t.Fatal("a 404 tag should error")
	}
	// The error is a friendly "no release tagged", driven by the errNotFound sentinel.
	if errors.Is(err, errNotFound) {
		t.Fatal("callers should map errNotFound to a message, not surface the sentinel")
	}
}

func TestFetchReleaseTagsViaGh(t *testing.T) {
	fakeGH(t)
	tags, err := fetchReleaseTags(time.Second)
	if err != nil || len(tags) != 2 || tags[0] != "v9.9.9" {
		t.Fatalf("tags via gh = %v, %v", tags, err)
	}
}

func TestGithubToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "abc")
	if githubToken() != "abc" {
		t.Errorf("should read GH_TOKEN, got %q", githubToken())
	}
	t.Setenv("GITHUB_TOKEN", "xyz")
	if githubToken() != "xyz" {
		t.Errorf("GITHUB_TOKEN should win, got %q", githubToken())
	}
}

// TestDebugGating: Logf is silent unless enabled, then writes a [debug] line.
func TestDebugGating(t *testing.T) {
	var buf bytesBuffer
	debug.SetOutput(&buf)
	t.Cleanup(func() { debug.SetOutput(os.Stderr); debug.SetEnabled(false) })

	debug.SetEnabled(false)
	debug.Logf("hidden")
	if buf.String() != "" {
		t.Errorf("disabled debug should emit nothing, got %q", buf.String())
	}
	debug.SetEnabled(true)
	debug.Logf("shown %d", 1)
	if got := buf.String(); got != "[debug] shown 1\n" {
		t.Errorf("enabled debug output = %q", got)
	}
}

// bytesBuffer is a tiny io.Writer capture (avoids importing bytes just for the test).
type bytesBuffer struct{ b []byte }

func (w *bytesBuffer) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
func (w *bytesBuffer) String() string              { return string(w.b) }
