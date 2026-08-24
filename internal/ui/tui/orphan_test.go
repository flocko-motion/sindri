package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// orphanAgentsModel is the Agents tab with the cursor on a stray container — no roster agent
// behind it — the row every one of these bindings silently did nothing on before agentSelected.
func orphanAgentsModel() model {
	m := newModel(nil, nil, "/r/one")
	m.tab = 1
	m.state = api.BoardState{Orphans: []string{"sindri-ghost"}}
	m.reclamp()
	return m
}

// TestAgentBindingsHideOnAnOrphanRow: eleven Agents-tab bindings dispatch through selAgent (or
// isOrphan) and silently do nothing on a stray container — no flash, no form — so each needs
// agentSelected naming that condition. The footer, the menu and the reference all generate from
// the same keymap, so fixing the row fixes all three at once.
func TestAgentBindingsHideOnAnOrphanRow(t *testing.T) {
	m := orphanAgentsModel()
	if id := m.selID(); id != "sindri-ghost" {
		t.Fatalf("precondition: cursor should be on the orphan, got %q", id)
	}
	footer := m.contextFooter()
	for _, gone := range []string{"tell: push now", "mail: waits", "attach", "editor"} {
		if strings.Contains(footer, gone) {
			t.Errorf("the footer should hide %q on an orphan row, got %q", gone, footer)
		}
	}
	for _, gone := range []string{"start/stop", "options", "milestone PR", "rebuild image", "rebase", "retire", "clear context"} {
		if menuHas(m, gone) {
			t.Errorf("the menu should not offer %q on an orphan row:\n%s", gone, menuText(m))
		}
	}
}

// TestAgentBindingsSurviveOnARosterAgent is the other half: the same row, a real agent this time,
// must still offer everything agentSelected gates — the fix must narrow only the orphan case.
func TestAgentBindingsSurviveOnARosterAgent(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab = 1
	m.state = api.BoardState{Agents: []api.AgentView{{Name: "dvalin", Project: "repo", Status: "idle"}}}
	m.reclamp()
	footer := m.contextFooter()
	for _, want := range []string{"tell: push now", "mail: waits", "attach", "editor"} {
		if !strings.Contains(footer, want) {
			t.Errorf("the footer should still offer %q on a roster agent, got %q", want, footer)
		}
	}
	for _, want := range []string{"start/stop", "options", "milestone PR", "rebuild image", "rebase", "retire", "clear context"} {
		if !menuHas(m, want) {
			t.Errorf("the menu should still offer %q on a roster agent:\n%s", want, menuText(m))
		}
	}
}

// TestAttachIsSilentNotBrokenOnAnOrphan pins the actual dispatch behaviour the finding was about:
// pressing a hidden key on an orphan does nothing, rather than panicking or attaching to nothing.
func TestAttachIsSilentNotBrokenOnAnOrphan(t *testing.T) {
	m := orphanAgentsModel()
	cmd := m.onKey(keyAttach)
	if cmd != nil {
		t.Error("attach on an orphan should produce no command")
	}
	if m.flash != "" {
		t.Errorf("attach on an orphan should not flash (the key is hidden, not offered-and-refused), got %q", m.flash)
	}
}
