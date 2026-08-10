package tui

import (
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
