package tui

import (
	"path/filepath"
	"testing"
)

// TestEditorPrefersTheUsersChoice: $VISUAL wins over $EDITOR, which wins over any system
// default — the order a Unix tool is expected to look in.
func TestEditorPrefersTheUsersChoice(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "sh") // on PATH everywhere, so it resolves
	if got := editorName(); got != "sh" {
		t.Errorf("$EDITOR should be used, got %q", got)
	}
	t.Setenv("VISUAL", "env")
	if got := editorName(); got != "env" {
		t.Errorf("$VISUAL should win over $EDITOR, got %q", got)
	}
}

// TestEditorSkipsWhatIsNotInstalled: a stale $EDITOR naming a since-removed editor must fall
// through to something that exists, rather than failing to open anything.
func TestEditorSkipsWhatIsNotInstalled(t *testing.T) {
	t.Setenv("VISUAL", "definitely-not-a-real-editor-9f3b")
	t.Setenv("EDITOR", "sh")
	if got := editorName(); got != "sh" {
		t.Errorf("an uninstalled $VISUAL should be skipped, got %q", got)
	}
}

// TestEditorCarriesBakedInArgs: $EDITOR is often a command with flags ("code --wait"), so the
// binary and its arguments must be split rather than exec'd as one impossible filename.
func TestEditorCarriesBakedInArgs(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "sh -c")
	cmd := editorAt("/tmp")
	if cmd == nil {
		t.Fatal("expected a command")
	}
	if filepath.Base(cmd.Path) != "sh" {
		t.Errorf("binary = %q, want sh", cmd.Path)
	}
	// The user's own flags, then the directory to open.
	if got := cmd.Args[len(cmd.Args)-2:]; got[0] != "-c" || got[1] != "." {
		t.Errorf("args = %v, want [… -c .]", cmd.Args)
	}
	if cmd.Dir != "/tmp" {
		t.Errorf("Dir = %q, want the worktree path", cmd.Dir)
	}
}

// TestEditorOpensTheDirectory: the point is to land in the PR's worktree, so the directory is
// passed as the argument — an editor with a file browser then shows the tree, not a blank buffer.
func TestEditorOpensTheDirectory(t *testing.T) {
	t.Setenv("VISUAL", "sh")
	cmd := editorAt("/some/worktree")
	if cmd == nil {
		t.Fatal("expected a command")
	}
	if cmd.Args[len(cmd.Args)-1] != "." {
		t.Errorf("last arg = %q, want \".\"", cmd.Args[len(cmd.Args)-1])
	}
	if cmd.Dir != "/some/worktree" {
		t.Errorf("Dir = %q", cmd.Dir)
	}
}
