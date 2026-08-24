package tui

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestSwitcherExcludesGlobalProject: GlobalProject is not a repo to switch into — it has no tasks
// of its own, and dialing it by its registered (real, hashed) path here would silently register a
// phantom project instead of reaching the real fleet-wide pool.
func TestSwitcherExcludesGlobalProject(t *testing.T) {
	m := newModel(nil, nil, "/r/sindri")
	m.state = api.BoardState{Projects: []api.Project{
		{Tag: "sin", Path: "/r/sindri"},
		{Tag: api.GlobalProject, Path: "/state/_global"},
	}}

	got := m.switcherOrder()
	if len(got) != 1 || got[0].Tag != "sin" {
		t.Errorf("switcherOrder = %+v, want only the real repo", got)
	}
}

// TestOpenSwitcherFlashesWhenOnlyGlobalIsRegistered: GlobalProject is always registered once the
// hub is up, so "no repos yet" must be judged after excluding it, not on the raw project count.
func TestOpenSwitcherFlashesWhenOnlyGlobalIsRegistered(t *testing.T) {
	m := newModel(nil, nil, "/r/sindri")
	m.state = api.BoardState{Projects: []api.Project{{Tag: api.GlobalProject, Path: "/state/_global"}}}

	m.openSwitcher()
	if m.flash != "no repos yet" {
		t.Errorf("flash = %q, want the no-repos message", m.flash)
	}
	if m.choice.active {
		t.Error("the switcher should not open with nothing but GlobalProject registered")
	}
}
