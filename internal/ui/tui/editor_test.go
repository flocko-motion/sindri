package tui

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/api"
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

// TestAgentWorkspacePathIsAbsolute: the path is handed to a child process as its working
// directory, and AgentView.Workspace is repo-relative — so it must be joined onto the agent's
// OWN repo. A relative path would resolve against wherever the TUI was launched, and the wrong
// repo's root would name a directory that exists but holds someone else's work.
func TestAgentWorkspacePathIsAbsolute(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.state = api.BoardState{
		Projects: []api.Project{
			{Tag: "one", Path: "/r/one"},
			{Tag: "two", Path: "/r/two"},
		},
		Agents: []api.AgentView{
			{Name: "dvalin", Project: "one", Workspace: ".worktrees/dvalin"},
			{Name: "alviss", Project: "two", Workspace: ".worktrees/alviss"}, // another repo
			{Name: "hepti", Project: "one", Workspace: "."},                  // coauthor: the checkout itself
		},
	}
	for _, tc := range []struct{ agent, want string }{
		{"dvalin", "/r/one/.worktrees/dvalin"},
		{"alviss", "/r/two/.worktrees/alviss"}, // its own project, not the selected one
		{"hepti", "/r/one"},
	} {
		if got := m.agentWorkspacePath(tc.agent); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.agent, got, tc.want)
		}
	}
	// An unknown agent, or one whose project the board doesn't carry, has no path to open.
	if got := m.agentWorkspacePath("nobody"); got != "" {
		t.Errorf("unknown agent should have no path, got %q", got)
	}
	m.state.Agents = append(m.state.Agents, api.AgentView{Name: "orphan", Project: "gone", Workspace: "x"})
	if got := m.agentWorkspacePath("orphan"); got != "" {
		t.Errorf("an agent whose repo is unknown has no resolvable path, got %q", got)
	}
}
