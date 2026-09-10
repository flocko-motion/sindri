package tui

import (
	"strings"
	"testing"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/agent/claude"
	"github.com/flo-at/sindri/internal/api"
)

// TestTheModelIsShownWithoutItsVendorPrefix: every id carries "claude-", so it distinguishes nothing
// while costing width in a column beside the context fill and the agent name. The BOARD's field is
// untouched — it is what the window lookup matches and what the dispatcher compares — so this asserts
// the rendering and not the state.
//
// The backend is wired here because it is wired in cmd/sindri: shortening is the coding agent's own
// knowledge of its ids, reached through the port, and against the no-op it is a pass-through.
func TestTheModelIsShownWithoutItsVendorPrefix(t *testing.T) {
	agentport.Use(claude.New())
	t.Cleanup(func() { agentport.Use(unreadableAgent{}) })

	m := newModel(nil, nil, "")
	m.tab, m.w, m.h = 1, 160, 40
	m.state = api.BoardState{
		Agents: []api.AgentView{{Name: "dain", Role: "worker", Status: "idle", Model: "claude-sonnet-5"}},
	}
	m.reclamp()

	for what, text := range map[string]string{
		"the agents row":  strings.Join(rowTexts(items(m.agentRows())), "\n"),
		"the detail pane": agentDetail(m),
	} {
		if strings.Contains(text, "claude-sonnet-5") {
			t.Errorf("%s still spells the vendor prefix out:\n%s", what, text)
		}
		if !strings.Contains(text, "sonnet-5") {
			t.Errorf("%s names no model at all:\n%s", what, text)
		}
	}
	// The board's own field is state, not presentation: shortened there, the window lookup and the
	// dispatcher's comparison would both be reading an id that no longer exists.
	if got := m.state.Agents[0].Model; got != "claude-sonnet-5" {
		t.Errorf("the board's model is %q, want the full id it arrived as", got)
	}
}

// unreadableAgent restores the port to the no-op a front-end test finds it in.
type unreadableAgent struct{ agentport.Agent }

func (unreadableAgent) DetectState(string) agentport.State { return agentport.Unknown }
func (unreadableAgent) ShortModel(m string) string         { return m }
