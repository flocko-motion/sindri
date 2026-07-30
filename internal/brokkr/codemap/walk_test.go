package codemap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestScanReportSeparatesEmptinesses is the whole point of the walk counting anything: "read
// nothing" and "read everything, matched nothing" used to be the same silence, and the first is
// not evidence of absence.
func TestScanReportSeparatesEmptinesses(t *testing.T) {
	// A tree with source, but not the language these tools parse.
	tsOnly := t.TempDir()
	if err := os.WriteFile(filepath.Join(tsOnly, "app.ts"), []byte("export const Target = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder
	if err := WriteRefs(&sb, []string{tsOnly}, -1, RefQuery{Symbol: "Target"}, 0); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, "no Go files under") || !strings.Contains(out, "read Go only") {
		t.Errorf("a non-Go tree must say it was never read, got:\n%s", out)
	}
	// The exact-identifier hint explains a MISS; offering it here would imply the tree was searched.
	if strings.Contains(out, "case-sensitive") {
		t.Errorf("an unread tree must not be explained as a near-miss, got:\n%s", out)
	}
	// And the old assertive claim must be gone: it was false for exactly this tree.
	if strings.Contains(out, "no references to Target") {
		t.Errorf("must not claim absence for a language it cannot read, got:\n%s", out)
	}
}

// TestScanReportOnAMapSearch: the same message shape covers map's searches, not just refs — one
// report for find/grep/symbol/refs instead of four near-duplicates.
func TestScanReportOnAMapSearch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := Write(&sb, []string{dir}, -1, Query{Grep: "nothinghere"}); err != nil {
		t.Fatal(err)
	}
	if out := sb.String(); !strings.Contains(out, "scanned 1 Go file") || !strings.Contains(out, "--grep") {
		t.Errorf("a map search that read files and matched none must say so, got:\n%s", out)
	}

	// A plain map of a non-Go tree: no pattern to name, but still not silence.
	empty := t.TempDir()
	sb.Reset()
	if err := Write(&sb, []string{empty}, -1, Query{}); err != nil {
		t.Fatal(err)
	}
	if out := sb.String(); !strings.Contains(out, "no Go files under") {
		t.Errorf("an empty map must say it read nothing, got:\n%s", out)
	}
}

// TestScanReportCoversEverySearch: the report lives in the shared walk, so all three map searches
// and refs get it from one place — the promise that adding a fourth search needs no fourth message.
func TestScanReportCoversEverySearch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		what  string
		query Query
		want  string
	}{
		{"--find", Query{Find: "nothinghere"}, "no match for --find"},
		{"--grep", Query{Grep: "nothinghere"}, "no match for --grep"},
		{"--symbol", Query{Symbol: "NothingHere"}, "no match for --symbol NothingHere"},
	} {
		var sb strings.Builder
		if err := Write(&sb, []string{dir}, -1, tc.query); err != nil {
			t.Fatal(err)
		}
		out := sb.String()
		if !strings.Contains(out, "scanned 1 Go file") || !strings.Contains(out, tc.want) {
			t.Errorf("%s should report the scan and name itself, got:\n%s", tc.what, out)
		}
	}
}

// TestPluralNeverReadsOneFiles guards the wording, which is the whole value of these messages.
func TestPluralNeverReadsOneFiles(t *testing.T) {
	if got := plural(1, "Go file"); got != "1 Go file" {
		t.Errorf("plural(1) = %q", got)
	}
	if got := plural(2, "Go file"); got != "2 Go files" {
		t.Errorf("plural(2) = %q", got)
	}
}
