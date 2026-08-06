package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
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

// TestLowercaseKeysNeverMutate pins the convention from the safe side. Every lowercase letter
// binding must be listed here with why it is harmless; anything else must be uppercase.
//
// The previous version allowlisted mutation LABELS and checked those were uppercase, which is
// fail-open: `merge` was never in the list, so `m` merging a PR on one keystroke passed for months.
// Inverted, a new binding is a violation until someone justifies it here.
func TestLowercaseKeysNeverMutate(t *testing.T) {
	safe := map[string]string{
		"a": "attach — hands the terminal to a session, changes nothing",
		"e": "edit — opens a form ($EDITOR on agents/prs); the form is what commits",
		"o": "open — a shell in the row's worktree",
		"t": "tell — opens a prompt you must submit",
		"i": "comment — opens a prompt you must submit; an empty one is refused",
		"c": "colour — opens a chooser",
		"f": "filter — narrows the view",
		"s": "scope — narrows the view",
		"p": "repo — switches the active repo",
		"m": "stats — samples memory use and shows it; mutates nothing",
		"r": "refresh — re-reads the board",
		"q": "quit",
	}
	m := newModel(nil, nil, "/r/one")
	for _, b := range keymap {
		r := []rune(b.keys)
		if len(r) != 1 || !unicode.IsLower(r[0]) || !unicode.IsLetter(r[0]) {
			continue
		}
		if _, ok := safe[b.keys]; !ok {
			t.Errorf("%q is lowercase but not declared harmless — either bind it uppercase or say why here (label: %q)", b.keys, b.label(m))
		}
	}
}

// TestMergeIsUppercase is the headline: merge commits on the keystroke — no form, no chooser —
// so it is the one action that most needs the shift. Spelled out, not read back from the constant.
func TestMergeIsUppercase(t *testing.T) {
	if keyMerge != "M" {
		t.Errorf("merge should be M, got %q", keyMerge)
	}
	if got := footerOf(t, scopePRs); !strings.Contains(got, "M merge") {
		t.Errorf("the PRs footer should offer \"M merge\":\n%s", got)
	}
}

// TestConfirmModalsDefaultToCancel: the cursor opens on the first option, so a destructive one
// there turns a stray Enter into the action. The approve-and-merge modal did exactly that.
func TestConfirmModalsDefaultToCancel(t *testing.T) {
	for _, tc := range []struct {
		what string
		open func(*model)
	}{
		{"approve & merge", func(m *model) { m.openApproveMergeChoice("pr-td-1") }},
		{"scrap PR", func(m *model) { m.openScrapPRChoice("pr-td-1") }},
		{"scrap task", func(m *model) { m.openScrapChoice("td-1") }},
		{"close task", func(m *model) { m.openCloseChoice("td-1", "pr-td-1") }},
		{"delete agent", func(m *model) { m.openDeleteChoice("dvalin") }},
		{"remove orphan", func(m *model) { m.openRemoveOrphanChoice("dvalin") }},
		{"forget repo", func(m *model) { m.openForgetChoice("one", "one") }},
		{"new meeting", func(m *model) { m.openNewMeetingChoice() }},
	} {
		m := newModel(nil, nil, "/r/one")
		tc.open(&m)
		if !m.choice.active {
			t.Errorf("%s: expected a modal", tc.what)
			continue
		}
		if len(m.choice.values) == 0 || m.choice.values[0] != "cancel" {
			t.Errorf("%s: cancel must be the default option, got %v", tc.what, m.choice.values)
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
	fleet := func() api.BoardState {
		return api.BoardState{
			Projects: []api.Project{{Tag: "repo", Path: "/r/one"}},
			Agents:   []api.AgentView{{Name: "dvalin", Project: "repo", Status: "idle", Workspace: ".worktrees/dvalin"}},
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
		st.PRs = []api.PR{{ID: "pr-td-1", Status: "open", Project: "repo", Agent: "dvalin", Branch: "td-1"}}
		m.tab, m.scopeRepo, m.state = 2, false, st
		if got := m.selWorktree(); got != want {
			t.Errorf("selWorktree() = %q, want %q", got, want)
		}
	})

	t.Run("prs tab: the agent is gone", func(t *testing.T) {
		m := newModel(nil, nil, "")
		st := fleet()
		st.PRs = []api.PR{{ID: "pr-td-9", Status: "open", Project: "repo", Agent: "vanished", Branch: "td-9"}}
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
		m.state = api.BoardState{PRs: []api.PR{{ID: "pr-td-1", Status: "open", Project: "repo", Branch: "td-1"}}}
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
		m.state = api.BoardState{Agents: []api.AgentView{{Name: "dvalin", Project: "repo", Status: "idle"}}}
		if id := m.selID(); id != "dvalin" {
			t.Fatalf("expected dvalin selected, got %q", id)
		}
		m.onKey(keyOptions)
		if !m.form.active || !strings.Contains(m.form.title, "dvalin") {
			t.Errorf("%q should open dvalin's options, got active=%v title=%q", keyOptions, m.form.active, m.form.title)
		}
	})

	t.Run("add member on the Meeting tab", func(t *testing.T) {
		m := newModel(nil, nil, "")
		m.tab = 4 // Meeting
		m.state = api.BoardState{
			Agents: []api.AgentView{{Name: "dvalin", Role: "worker"}, {Name: "nori", Role: "reviewer"}},
			Chat:   api.ChatView{Members: []api.ChatMember{{Name: "nori", Role: "reviewer"}}},
		}
		m.onKey(keyApprove)
		if !m.choice.active {
			t.Fatal("A should open the add-member chooser")
		}
		if len(m.choice.values) != 1 || m.choice.values[0] != "dvalin" {
			t.Errorf("the add-member chooser should offer only agents not already in the room, got %v", m.choice.values)
		}
	})

	t.Run("remove member on the Meeting tab", func(t *testing.T) {
		m := newModel(nil, nil, "")
		m.tab = 4 // Meeting
		m.state = api.BoardState{Chat: api.ChatView{Members: []api.ChatMember{{Name: "nori", Role: "reviewer"}}}}
		m.onKey(keyReject)
		if !m.choice.active {
			t.Fatal("R should open the remove-member chooser")
		}
		if len(m.choice.values) != 1 || m.choice.values[0] != "nori" {
			t.Errorf("the remove-member chooser should offer the current roster, got %v", m.choice.values)
		}
	})
}

// TestMeetingMembershipKeysHaveNothingToOffer: A/R degrade to a flash rather than an empty
// chooser when there is nobody to add or remove — a modal with zero rows is a dead end.
func TestMeetingMembershipKeysHaveNothingToOffer(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 4 // Meeting
	m.state = api.BoardState{Agents: []api.AgentView{{Name: "nori"}}, Chat: api.ChatView{Members: []api.ChatMember{{Name: "nori"}}}}
	m.onKey(keyApprove) // every agent is already a member
	if m.choice.active {
		t.Error("A should not open a chooser with nothing to add")
	}
	if m.flash == "" {
		t.Error("A should flash why there was nothing to add")
	}

	m = newModel(nil, nil, "")
	m.tab = 4
	m.onKey(keyReject) // empty room
	if m.choice.active {
		t.Error("R should not open a chooser with nothing to remove")
	}
	if m.flash == "" {
		t.Error("R should flash why there was nothing to remove")
	}
}

// TestMeetingMembershipKeysAreAdvertised: the Chat footer must offer A/R now that the gap the
// old comment described ("membership is curated from the CLI") is closed.
func TestMeetingMembershipKeysAreAdvertised(t *testing.T) {
	got := footerOf(t, scopeChat)
	if !strings.Contains(got, keyApprove+" add member") {
		t.Errorf("the Meeting footer should offer %q add member:\n%s", keyApprove, got)
	}
	if !strings.Contains(got, keyReject+" remove member") {
		t.Errorf("the Meeting footer should offer %q remove member:\n%s", keyReject, got)
	}
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

// TestAgentsTabReachesMilestoneRebuildAndStats pins the three actions that used to be CLI-only. It
// asserts the whole path, not just the constant: the key is advertised on the Agents footer, and
// pressing it there gets somewhere — a cancel-first chooser for the two mutations, a command for the
// view. The client is a zero-value one so the branches are not short-circuited by a nil check; the
// commands are never run, so nothing dials.
func TestAgentsTabReachesMilestoneRebuildAndStats(t *testing.T) {
	footer := footerOf(t, scopeAgents)
	for _, tc := range []struct {
		key, label string
		mutates    bool
	}{
		{keyMilestone, "milestone PR", true},
		{keyRebuild, "rebuild image", true},
		{keyStats, "stats", false},
	} {
		if !strings.Contains(footer, tc.key+" "+tc.label) {
			t.Errorf("%q (%s) is not advertised on the Agents footer: %s", tc.key, tc.label, footer)
		}
		m := newModel(&client.HTTP{}, nil, "")
		m.tab, m.scopeRepo = 1, false
		m.state = api.BoardState{Agents: []api.AgentView{{Name: "dvalin", Project: "repo", Status: "idle"}}}
		cmd := m.onKey(tc.key)

		if !tc.mutates {
			if m.choice.active {
				t.Errorf("%q is a view; it should not ask for confirmation", tc.key)
			}
			if cmd == nil {
				t.Errorf("%q produced no command — the binding reaches nothing", tc.key)
			}
			continue
		}
		// A mutation must land on a chooser and never fire off the keystroke itself.
		if !m.choice.active {
			t.Fatalf("%q did not open a confirmation", tc.key)
		}
		if cmd != nil {
			t.Errorf("%q returned a command as well as a confirmation — it should wait for the answer", tc.key)
		}
		if !strings.Contains(m.choice.title, "dvalin") {
			t.Errorf("%q asks about no agent in particular: %q", tc.key, m.choice.title)
		}
		if m.choice.values[0] != "cancel" {
			t.Errorf("%q defaults to %q, want cancel first", tc.key, m.choice.values[0])
		}
		if m.choice.apply("cancel") != nil {
			t.Errorf("%q acts on cancel", tc.key)
		}
		if m.choice.apply(m.choice.values[1]) == nil {
			t.Errorf("%q confirmed but produced no command", tc.key)
		}
	}
}

// TestStatsLinesReportTheReasonNotAZero guards the failure this whole family of views keeps getting
// wrong: an agent the engine could not sample must say so, not render 0 B and read as idle.
func TestStatsLinesReportTheReasonNotAZero(t *testing.T) {
	out := strings.Join(statsLines(api.StatsReport{
		Engine: "podman",
		Agents: []api.AgentStatsView{{Name: "bombur", Repo: "sindri", Err: "container not found"}},
	}), "\n")
	if !strings.Contains(out, "container not found") {
		t.Errorf("the sampling error is not shown:\n%s", out)
	}
	if strings.Contains(out, "0 B") {
		t.Errorf("an unsampled agent renders as 0 B, which reads as idle:\n%s", out)
	}
	// An empty fleet must say it is empty rather than print a bare header.
	if empty := strings.Join(statsLines(api.StatsReport{Engine: "podman"}), "\n"); !strings.Contains(empty, "no running agents") {
		t.Errorf("an empty report says nothing:\n%s", empty)
	}
}
