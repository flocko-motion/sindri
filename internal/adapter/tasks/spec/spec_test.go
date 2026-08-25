package spec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Version degrades to ok=false rather than an error or a panic when the CLI isn't on PATH — the
// shape a best-effort host-side version probe (hosttools.Versions) needs.
func TestVersionMissingCLI(t *testing.T) {
	t.Setenv("PATH", "")
	if _, ok := Version(context.Background()); ok {
		t.Fatal("a missing CLI must report ok=false, not a version")
	}
}

// A CLI whose own --version prints more than one line must compare against only its first: the
// pod-side manifest only ever captures the single build-log line its marker echo produced, so
// keeping a trailing banner here would make this tool a permanent, unactionable mismatch.
func TestVersionKeepsOnlyTheFirstLine(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "openspec")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '1.8.0\\nsome extra banner line\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	v, ok := Version(context.Background())
	if !ok || v != "1.8.0" {
		t.Errorf("Version() = (%q, %v), want (\"1.8.0\", true)", v, ok)
	}
}

func TestValidateNoOpenspecDirSkipsSilently(t *testing.T) {
	ok, out := Validate(t.TempDir())
	if !ok || out != "" {
		t.Fatalf("a project without openspec/ should skip silently, got ok=%v out=%q", ok, out)
	}
}

func TestValidateMissingCLIDegradesVisibly(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "openspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "") // hide the openspec CLI
	ok, out := Validate(dir)
	if !ok {
		t.Fatal("a missing optional CLI must not be a validation failure")
	}
	if !strings.Contains(out, "not installed") {
		t.Errorf("the skip must be visible, got: %q", out)
	}
}

// TestProposalIsTheDescription: an openspec change is a prose document — the proposal IS
// its description. `openspec list --json` yields only a name and task counts, so a spec
// task used to reach the board with nothing to read, which left the one meaningful part of
// the change out of the only view that shows it.
func TestProposalIsTheDescription(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "openspec", "changes", "add-widgets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const body = "## Why\n\nWidgets are needed.\n\n## What changes\n\n- add a widget"
	if err := os.WriteFile(filepath.Join(dir, "proposal.md"), []byte("# Add widgets\n\n"+body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := Proposal(root, "add-widgets")
	if got != body {
		t.Errorf("proposal =\n%q\nwant\n%q", got, body)
	}
	// The title column already carries the change name, so the leading heading is dropped.
	if strings.Contains(got, "# Add widgets") {
		t.Errorf("the leading heading should be skipped, got:\n%s", got)
	}
}

// TestProposalMissingIsEmptyNotFatal: design.md and tasks.md can stand alone, so a change
// without a proposal is legitimate — it must yield no description rather than break the
// whole task listing.
func TestProposalMissingIsEmptyNotFatal(t *testing.T) {
	if got := Proposal(t.TempDir(), "nope"); got != "" {
		t.Errorf("a missing proposal should be empty, got %q", got)
	}
}

// TestProposalRejectsPathEscape: the change name comes from openspec's output and is used
// to build a path, so it must not be able to walk out of the changes directory.
func TestProposalRejectsPathEscape(t *testing.T) {
	for _, bad := range []string{"../../etc", "a/b", "..", "."} {
		if got := Proposal(t.TempDir(), bad); got != "" {
			t.Errorf("name %q should be refused, got %q", bad, got)
		}
	}
}

// TestFormatReportEmptyProjectSaysSo is the reported bug: an openspec/ directory with nothing in it
// yet is a valid, passing, EMPTY report — and it took the unparseable branch, dumping the raw JSON
// blob into every `brokkr lint` run.
func TestFormatReportEmptyProjectSaysSo(t *testing.T) {
	raw := []byte(`{"items":[],"summary":{"totals":{"items":0,"passed":0,"failed":0}},"version":"1.0"}`)
	got := formatReport(raw, false)
	if strings.Contains(got, "{") || strings.Contains(got, "totals") {
		t.Errorf("a valid empty report must not dump its JSON, got:\n%s", got)
	}
	if !strings.Contains(got, "no specs or changes") {
		t.Errorf("it should say there was nothing to validate, got: %q", got)
	}
}

// TestFormatReportPassIsOneLine: a pass needs its verdict — proof the delegated validator ran — not
// a report of everything that was fine.
func TestFormatReportPassIsOneLine(t *testing.T) {
	raw := []byte(`{"items":[{"id":"a","type":"spec","valid":true},{"id":"b","type":"change","valid":true}]}`)
	got := formatReport(raw, false)
	if lines := strings.Count(strings.TrimSpace(got), "\n") + 1; lines != 1 {
		t.Errorf("a pass should be one line, got %d:\n%s", lines, got)
	}
	if !strings.Contains(got, "2 passed") {
		t.Errorf("the verdict should carry the count, got: %q", got)
	}
}

// TestFormatReportFailureKeepsTheDetail: the compact pass must not have cost the report you need
// when something is actually wrong — the failing item, its file and the rule.
func TestFormatReportFailureKeepsTheDetail(t *testing.T) {
	raw := []byte(`{"items":[
		{"id":"good","type":"spec","valid":true},
		{"id":"bad","type":"change","valid":false,"issues":[{"path":"openspec/changes/bad/proposal.md","message":"missing Why","level":"ERROR"}]}
	]}`)
	got := formatReport(raw, true)
	for _, want := range []string{"bad", "missing Why", "openspec/changes/bad/proposal.md", "1 passed, 1 failed"} {
		if !strings.Contains(got, want) {
			t.Errorf("a failing report should mention %q, got:\n%s", want, got)
		}
	}
}

// TestFormatReportUnparseableStillShows: an unexpected shape is shown rather than swallowed — that
// branch is why the empty report was being dumped, and it still has to work for its real case.
func TestFormatReportUnparseableStillShows(t *testing.T) {
	raw := []byte(`not json at all`)
	if got := formatReport(raw, true); !strings.Contains(got, "not json at all") {
		t.Errorf("unparseable output must survive, got: %q", got)
	}
}

// TestChangedAtDatesAChangeFromItsFiles: openspec keeps no timestamps, so an os- task reached the
// board undated — and an undated task can never be recent, which kept every finished change out of
// the "active" filter that exists to show work just completed. The files are the record: ticking a
// box in tasks.md is the change changing.
func TestChangedAtDatesAChangeFromItsFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "openspec", "changes", "add-widgets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"proposal.md", "tasks.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "proposal.md"), old, old); err != nil {
		t.Fatal(err)
	}

	got := changedAt(root, "add-widgets")
	if got.IsZero() {
		t.Fatal("a change on disk must carry a date; undated is what put it outside every recency filter")
	}
	// The NEWEST file wins: a proposal written days ago says nothing about a box ticked minutes ago.
	if time.Since(got) > time.Hour {
		t.Errorf("changedAt = %v, want the newest file's time (tasks.md, just written)", got)
	}
}

// TestChangedAtRejectsPathEscape: same guard as Proposal — a name is one path segment, never a walk
// out of the changes directory.
func TestChangedAtRejectsPathEscape(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../../etc"} {
		if got := changedAt(t.TempDir(), name); !got.IsZero() {
			t.Errorf("changedAt(%q) = %v, want the zero time", name, got)
		}
	}
}
