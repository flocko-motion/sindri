package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

// scopeLabels is what a tab's footer effectively offers: its own bindings plus the global ones,
// which show on every tab. Compound display rows ("j/k", "A/R") are split, and any part that
// isn't a single key ("[]", "C-h", "enter") is dropped — those are prose, not dispatchable keys.
func scopeLabels(t *testing.T, scope keyScope) map[string][]string {
	t.Helper()
	m := newModel(nil, nil, "/r/one")
	out := map[string][]string{}
	for _, b := range keymap {
		if b.scope != scope && b.scope != scopeGlobal {
			continue
		}
		for _, part := range strings.Split(b.keys, "/") {
			if len([]rune(part)) == 1 {
				out[part] = append(out[part], b.label(m))
			}
		}
	}
	return out
}

// TestNoTwoActionsShareAKeyOnATab is the guard keys.go claims to be: one key, one meaning per
// tab. The same key with the same label twice is only a duplicated help row (config is listed
// globally and on Repos), so labels — not counts — decide what a conflict is.
func TestNoTwoActionsShareAKeyOnATab(t *testing.T) {
	for _, scope := range []keyScope{scopeGlobal, scopeTasks, scopeAgents, scopePRs, scopeRepos, scopeChat} {
		for key, labels := range scopeLabels(t, scope) {
			for _, l := range labels {
				if l != labels[0] {
					t.Errorf("scope %d: key %q means both %q and %q", scope, key, labels[0], l)
				}
			}
		}
	}
}

// TestMutationsAreUppercase pins the convention: a lowercase key looks or navigates, so it can
// never be the one that changes something. Approve is the case that prompted it — it used to be
// `a`, one slip away from attach. Merge is the standing exception (long-standing muscle memory),
// so it is named here rather than silently tolerated.
func TestMutationsAreUppercase(t *testing.T) {
	mutations := map[string]bool{
		"approve": true, "reject": true, "agent-review": true, "scrap": true,
		"delete": true, "new": true, "priority": true, "unassign": true,
		"close": true, "options": true, "rebase": true, "forget": true,
	}
	m := newModel(nil, nil, "/r/one")
	for _, b := range keymap {
		label := b.label(m)
		if !mutations[label] || len([]rune(b.keys)) != 1 {
			continue
		}
		if r := []rune(b.keys)[0]; !unicode.IsUpper(r) {
			t.Errorf("%q mutates but is bound to lowercase %q", label, b.keys)
		}
	}
}

// TestEditorIsBoundToE: the editor opens with `e`, the letter it starts with, on every tab that
// offers it — `o` was a mnemonic for nothing, and now opens a shell instead.
func TestEditorIsBoundToE(t *testing.T) {
	for _, scope := range []keyScope{scopePRs, scopeAgents} {
		footer := footerOf(t, scope)
		if !strings.Contains(footer, keyEdit+" editor") {
			t.Errorf("scope %d should open the editor with %q:\n%s", scope, keyEdit, footer)
		}
		if strings.Contains(footer, "o editor") {
			t.Errorf("scope %d still offers the old editor key:\n%s", scope, footer)
		}
	}
}

// TestPRTabVerdictKeys: the three verdicts a human hands down from the PRs tab, each on its own
// key. Approve on A (not a, where attach lives on the other tabs) and the agentic review moved
// off A to make room, so pin all of it — this is the arrangement the tab was wrong about.
// The letters are spelled out, not built from the constants: a test that reads the constant back
// to itself would pass whatever the constant became, which is the regression to catch.
func TestPRTabVerdictKeys(t *testing.T) {
	footer := footerOf(t, scopePRs)
	for _, want := range []string{"A approve", "R reject", "e editor"} {
		if !strings.Contains(footer, want) {
			t.Errorf("the PRs footer should offer %q:\n%s", want, footer)
		}
	}
	// The agentic review keeps A's old job but not its key, and must stay out of lowercase.
	if keyReview == "A" || strings.ToUpper(keyReview) != keyReview {
		t.Errorf("agent-review should be its own uppercase key, got %q", keyReview)
	}
	if !strings.Contains(footer, keyReview+" agent-review") {
		t.Errorf("the PRs footer should offer %q agent-review:\n%s", keyReview, footer)
	}
}

// TestOldKeysLostTheirOldJobs: a rebound key must not keep what it used to do. `o` was the editor
// and is now open-in-shell; `a` keeps only attach, never approve — mistyping attach into an
// approval is the danger this rebinding removes.
func TestOldKeysLostTheirOldJobs(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	for _, b := range keymap {
		if b.keys == keyOpen && b.label(m) != "open" {
			t.Errorf("`%s` opens a shell now, but it is bound to %q", keyOpen, b.label(m))
		}
		if b.keys == keyAttach && b.label(m) != "attach" {
			t.Errorf("`%s` must only attach, but it is bound to %q", keyAttach, b.label(m))
		}
	}
}

// TestOpenShellTargetsTheWorktree: `o` opens the tree the row stands for — the agent's own on the
// Agents tab, and on the PRs tab the tree of the agent that AUTHORED the PR (not the selection on
// some other tab). A PR outlives its agent, so the case with no tree left has to say so.
func TestOpenShellTargetsTheWorktree(t *testing.T) {
	fleet := func() hub.BoardState {
		return hub.BoardState{
			Projects: []store.Project{{Tag: "repo", Path: "/r/one"}},
			Agents:   []hub.AgentView{{Name: "dvalin", Project: "repo", Status: "idle", Workspace: ".worktrees/dvalin"}},
		}
	}
	want := filepath.Join("/r/one", ".worktrees", "dvalin")

	t.Run("agents tab: the selected agent's own tree", func(t *testing.T) {
		m := newModel(nil, nil, "")
		m.tab, m.scopeRepo, m.state = 1, false, fleet()
		if got := m.selWorktree(); got != want {
			t.Errorf("selWorktree() = %q, want %q", got, want)
		}
	})

	t.Run("prs tab: the authoring agent's tree", func(t *testing.T) {
		m := newModel(nil, nil, "")
		st := fleet()
		st.PRs = []store.PR{{ID: "pr-td-1", Status: "open", Project: "repo", Agent: "dvalin", Branch: "td-1"}}
		m.tab, m.scopeRepo, m.state = 2, false, st
		if got := m.selWorktree(); got != want {
			t.Errorf("selWorktree() = %q, want %q", got, want)
		}
	})

	t.Run("prs tab: the agent is gone", func(t *testing.T) {
		m := newModel(nil, nil, "")
		st := fleet()
		st.PRs = []store.PR{{ID: "pr-td-9", Status: "open", Project: "repo", Agent: "vanished", Branch: "td-9"}}
		m.tab, m.scopeRepo, m.state = 2, false, st
		if got := m.selWorktree(); got != "" {
			t.Errorf("a vanished agent leaves no tree, got %q", got)
		}
		m.onKey(keyOpen)
		if !strings.Contains(m.flash, "no worktree") {
			t.Errorf("the key should say there is no tree, flash = %q", m.flash)
		}
	})
}

// TestNewKeysReachTheirActions drives the dispatcher, not just the help: the two rebound actions
// that open a form are observable headlessly, so pressing the new key must open exactly that
// form. The rest of the tab's actions call the hub, which needs a live one.
func TestNewKeysReachTheirActions(t *testing.T) {
	t.Run("agent-review on the PRs tab", func(t *testing.T) {
		m := newModel(nil, nil, "")
		m.tab, m.scopeRepo = 2, false
		m.state = hub.BoardState{PRs: []store.PR{{ID: "pr-td-1", Status: "open", Project: "repo", Branch: "td-1"}}}
		if id := m.selID(); id != "pr-td-1" {
			t.Fatalf("expected pr-td-1 selected, got %q", id)
		}
		m.onKey(keyReview)
		if !m.form.active || !strings.Contains(m.form.title, "agent-review of pr-td-1") {
			t.Errorf("%q should open the agent-review form, got active=%v title=%q", keyReview, m.form.active, m.form.title)
		}
	})

	t.Run("options on the Agents tab", func(t *testing.T) {
		m := newModel(nil, nil, "")
		m.tab, m.scopeRepo = 1, false
		m.state = hub.BoardState{Agents: []hub.AgentView{{Name: "dvalin", Project: "repo", Status: "idle"}}}
		if id := m.selID(); id != "dvalin" {
			t.Fatalf("expected dvalin selected, got %q", id)
		}
		m.onKey(keyOptions)
		if !m.form.active || !strings.Contains(m.form.title, "dvalin") {
			t.Errorf("%q should open dvalin's options, got active=%v title=%q", keyOptions, m.form.active, m.form.title)
		}
	})
}

// footerOf renders one scope's footer for assertions — the string the user actually reads, and
// the only place a binding is advertised.
func footerOf(t *testing.T, scope keyScope) string {
	t.Helper()
	return newModel(nil, nil, "/r/one").footerFor(scope)
}

// TestNewMeetingKeyIsOfferedAndConfirmed: N means "new" on every tab that has one, and on the
// Meeting tab it clears history for everyone — so it must be advertised, and it must ask first.
func TestNewMeetingKeyIsOfferedAndConfirmed(t *testing.T) {
	if got := footerOf(t, scopeChat); !strings.Contains(got, keyNew+" new meeting") {
		t.Errorf("the Meeting footer should offer %q:\n%s", keyNew, got)
	}

	m := newModel(nil, nil, "")
	m.tab = 4 // Meeting
	m.onKey(keyNew)
	if !m.choice.active {
		t.Fatal("N must confirm before clearing the shared history, not act immediately")
	}
	if !strings.Contains(m.choice.title, "new meeting") {
		t.Errorf("the confirm should name what it does, got %q", m.choice.title)
	}
	// Cancel is first, so a stray Enter on the modal cannot wipe the room.
	if len(m.choice.values) == 0 || m.choice.values[0] != "cancel" {
		t.Errorf("cancel must be the default option, got %v", m.choice.values)
	}
}
