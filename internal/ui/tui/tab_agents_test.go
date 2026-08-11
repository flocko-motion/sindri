package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestAgentDetailShowsTheSameFieldsAsAgentInfo pins the item-detail-parity rule for an agent: the
// two front-ends show the same item, so they must show the same fields. `sindri agent info` prints
// these labels; the TUI detail must carry them too. Feature and the liveness probe were the gap —
// info printed them and the detail did not.
func TestAgentDetailShowsTheSameFieldsAsAgentInfo(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.scopeRepo = 1, false
	m.state = api.BoardState{Agents: []api.AgentView{{
		Name: "dvalin", Role: "worker", Status: "idle", Task: "td-1", Feature: "td-parent",
		PR: "pr-1", Workspace: "/w/dvalin",
	}}}
	joined := strings.Join(itemTexts(m.agentItems()), "\n")
	for _, label := range []string{"role:", "status:", "task:", "feature:", "pr:", "workspace:", "memory:", "container:"} {
		if !strings.Contains(joined, label) {
			t.Errorf("the agent detail omits %q, which `agent info` prints:\n%s", label, joined)
		}
	}
	// The feature must be a cross-reference, not just text: it names a task you can open.
	var feat metaItem
	for _, it := range m.agentItems() {
		if strings.HasPrefix(it.text, "feature:") {
			feat = it
		}
	}
	if feat.kind != "task" || feat.value != "td-parent" {
		t.Errorf("the feature line should open its task, got kind=%q value=%q", feat.kind, feat.value)
	}
}

// TestStatusOpensTheLivenessProbe: `agent info --debug` explains a puzzling status; the TUI reaches
// the same explanation by selecting the status line, so no hotkey is spent and the question is asked
// where it arises. Selecting it again returns to the live screen.
func TestStatusOpensTheLivenessProbe(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.scopeRepo = 1, false
	m.state = api.BoardState{Agents: []api.AgentView{{Name: "dvalin", Status: "idle"}}}

	var status metaItem
	for _, it := range m.agentItems() {
		if strings.HasPrefix(it.text, "status:") {
			status = it
		}
	}
	if status.kind != "view" || status.value != "diag" {
		t.Fatalf("the status line should be selectable as a view, got kind=%q value=%q", status.kind, status.value)
	}
	m.agentView = "diag"
	m.agentDiag = "" // the fetch has not landed yet
	if body := strings.Join(m.paneLines(), "\n"); !strings.Contains(body, "asking the hub") {
		t.Errorf("a pending probe should say so, not render blank: %q", body)
	}
	m.agentDiag = "tmux: session present\nclaude: running"
	body := strings.Join(m.paneLines(), "\n")
	if !strings.Contains(body, "claude: running") {
		t.Errorf("the probe output is not shown: %q", body)
	}
	if !strings.Contains(body, "idle") {
		t.Errorf("the probe should say which status it explains: %q", body)
	}
}

// TestAgentWorkspaceIsAFocusablePath: the PRs tab has long offered the authoring agent's tree as a
// "path" item, which is the single classification that makes it focusable, openable with ENTER and
// copyable with `y`. The Agents tab named the same tree in plain text, so the tab that is ABOUT the
// agent was the one place you could not take its path anywhere.
func TestAgentWorkspaceIsAFocusablePath(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.scopeRepo = 1, false
	m.state = api.BoardState{
		Projects: []api.Project{{Tag: "repo", Path: "/r/one"}},
		Agents:   []api.AgentView{{Name: "dvalin", Project: "repo", Status: "idle", Workspace: ".worktrees/dvalin"}},
	}
	want := filepath.Join("/r/one", ".worktrees", "dvalin")

	var ws metaItem
	for _, it := range m.agentItems() {
		if strings.HasPrefix(it.text, "workspace:") {
			ws = it
		}
	}
	if ws.kind != "path" || ws.value != want {
		t.Fatalf("the workspace line should be an openable path, got kind=%q value=%q", ws.kind, ws.value)
	}
	// Absolute in the text too: `y` copies the value, so a shorter text would copy something the
	// screen never showed.
	if !strings.Contains(ws.text, want) {
		t.Errorf("the workspace line shows %q, but %q is what it copies and opens", ws.text, want)
	}
	// Focusable means present in the actionable subset — that list is what the right cursor walks.
	var found bool
	for _, it := range m.agentActionable() {
		if it.kind == "path" && it.value == want {
			found = true
		}
	}
	if !found {
		t.Error("the workspace path is not reachable by the right-column cursor")
	}

	// And `y` on it copies the path rather than the row id, which is the whole point of the ask.
	for i, it := range m.agentActionable() {
		if it.kind == "path" {
			m.rightFocus, m.rightCursor = true, i
		}
	}
	m.onKey("y")
	if m.flash != "copied: "+want {
		t.Errorf("`y` on the workspace reported %q, want the path copied", m.flash)
	}
}

// TestAgentWorkspaceWithoutAProjectRootStaysPlain: the absolute path is built by joining the agent's
// repo-relative workspace to its project, so an unregistered project leaves nothing to join. The
// field still shows — a field that vanishes reads as a bug — but not as somewhere to open, since a
// shell started at a relative path lands wherever the TUI happens to be running.
func TestAgentWorkspaceWithoutAProjectRootStaysPlain(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.scopeRepo = 1, false
	m.state = api.BoardState{Agents: []api.AgentView{{Name: "dvalin", Status: "idle", Workspace: ".worktrees/dvalin"}}}

	var ws metaItem
	for _, it := range m.agentItems() {
		if strings.HasPrefix(it.text, "workspace:") {
			ws = it
		}
	}
	if !strings.Contains(ws.text, ".worktrees/dvalin") {
		t.Errorf("the workspace field disappeared when its project root was unknown: %q", ws.text)
	}
	if ws.kind != "" {
		t.Errorf("a workspace with no root to join to was offered as a %q to open", ws.kind)
	}
}
