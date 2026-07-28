package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
