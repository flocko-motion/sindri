package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
)

// TestTasksJumpToNeedingUser: ] and [ walk to the next/previous VISIBLE task that needs the user —
// read off api.TaskNeedsUser, the same predicate the badge and the red row use (sd-57e895). Task
// priority sorts non-empty-priority tasks first, then by id, so this fixture's rendered order is
// deterministic: t-done-1, t-done-2 (both rated, approved), then t-pending-a, t-pending-b (both
// unrated, so unreleased — needing a verdict regardless).
func TestTasksJumpToNeedingUser(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 0
	m.w, m.h = 120, 40
	m.state = api.BoardState{Tasks: []api.Task{
		{ID: "t-done-1", Title: "not needing", Status: "open", Approval: "approved", Priority: "P2"},
		{ID: "t-pending-a", Title: "needs a verdict", Status: "open", Approval: "pending"},
		{ID: "t-done-2", Title: "also not needing", Status: "open", Approval: "approved", Priority: "P2"},
		{ID: "t-pending-b", Title: "needs a verdict too", Status: "open", Approval: "pending"},
	}}
	m.reclamp()

	rows := m.rows()
	var ids []string
	for _, r := range items(rows) {
		ids = append(ids, r.id)
	}
	want := []string{"t-done-1", "t-done-2", "t-pending-a", "t-pending-b"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("precondition: rendered order = %v, want %v", ids, want)
	}

	m.cursor[0] = 0 // on t-done-1
	m.onKey("]")
	if got := m.selID(); got != "t-pending-a" {
		t.Fatalf("] from t-done-1 landed on %q, want t-pending-a", got)
	}
	m.onKey("]")
	if got := m.selID(); got != "t-pending-b" {
		t.Fatalf("] again landed on %q, want t-pending-b", got)
	}
	m.onKey("]") // past the last match: no cycling back to t-pending-a
	if got := m.selID(); got != "t-pending-b" {
		t.Errorf("] past the last needing row moved to %q — it must not cycle", got)
	}
	if m.flash == "" {
		t.Error("] past the last needing row should flash rather than silently do nothing")
	}

	m.onKey("[")
	if got := m.selID(); got != "t-pending-a" {
		t.Fatalf("[ from t-pending-b landed on %q, want t-pending-a", got)
	}
	m.onKey("[") // past the first match: no cycling to t-pending-b
	if got := m.selID(); got != "t-pending-a" {
		t.Errorf("[ past the first needing row moved to %q — it must not cycle", got)
	}
	if m.flash == "" {
		t.Error("[ past the first needing row should flash rather than silently do nothing")
	}
}

// TestTasksJumpSkipsWhatIsNotOnScreen: a needing row hidden by the active search, or folded under a
// collapsed parent, is not a candidate — the cursor must land only where the user can actually see it.
func TestTasksJumpSkipsWhatIsNotOnScreen(t *testing.T) {
	t.Run("search", func(t *testing.T) {
		m := newModel(nil, nil, "")
		m.tab = 0
		m.w, m.h = 120, 40
		m.state = api.BoardState{Tasks: []api.Task{
			{ID: "t-visible", Title: "visible one", Status: "open", Approval: "approved", Priority: "P2"},
			{ID: "t-hidden-pending", Title: "excluded by search", Status: "open", Approval: "pending"},
		}}
		m.taskSearch = "visible" // narrows to t-visible; t-hidden-pending's title does not match
		m.reclamp()

		if n := itemRows(m.rows()); n != 1 {
			t.Fatalf("precondition: the search should leave exactly one selectable row, got %d", n)
		}
		m.cursor[0] = 0
		m.onKey("]")
		if got := m.selID(); got != "t-visible" {
			t.Errorf("] should not jump to a task the search hides, cursor moved to %q", got)
		}
		if m.flash == "" {
			t.Error("] with nothing reachable should still flash, not silently do nothing")
		}
	})

	t.Run("collapsed fold", func(t *testing.T) {
		m := newModel(nil, nil, "")
		m.tab = 0
		m.w, m.h = 120, 40
		m.state = api.BoardState{Tasks: []api.Task{
			{ID: "t-parent", Title: "parent", Status: "open", Approval: "approved", Priority: "P2"},
			{ID: "t-child-pending", Title: "child needing a verdict", Status: "open", Approval: "pending", ParentID: "t-parent"},
		}}
		m.collapsed["t-parent"] = true
		m.reclamp()

		if n := itemRows(m.rows()); n != 1 {
			t.Fatalf("precondition: the fold should hide the child, got %d selectable rows", n)
		}
		m.cursor[0] = 0
		m.onKey("]")
		if got := m.selID(); got != "t-parent" {
			t.Errorf("] should not jump to a child folded out of view, cursor moved to %q", got)
		}
	})
}

// TestAgentsJumpToNeedingUser mirrors the Tasks case for api.AgentNeedsUser, on agents sorted (same
// project/role) by name: a-idle, b-blocked, c-idle, d-escalated.
func TestAgentsJumpToNeedingUser(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.scopeRepo = 1, false
	m.w, m.h = 120, 40
	m.state = api.BoardState{
		Projects: []api.Project{{Tag: "repo", Path: "/r/one"}},
		Agents: []api.AgentView{
			{Name: "a-idle", Project: "repo", Role: "worker", Status: "idle"},
			{Name: "b-blocked", Project: "repo", Role: "worker", Status: api.StatusBlocked, NeedsUser: true},
			{Name: "c-idle", Project: "repo", Role: "worker", Status: "idle"},
			{Name: "d-escalated", Project: "repo", Role: "worker", Status: api.StatusEscalated, NeedsUser: true},
		},
	}
	m.reclamp()
	m.selectRow("a-idle")

	m.onKey("]")
	if got := m.selID(); got != "b-blocked" {
		t.Fatalf("] landed on %q, want b-blocked", got)
	}
	m.onKey("]")
	if got := m.selID(); got != "d-escalated" {
		t.Fatalf("] again landed on %q, want d-escalated", got)
	}
	m.onKey("[")
	if got := m.selID(); got != "b-blocked" {
		t.Fatalf("[ back landed on %q, want b-blocked", got)
	}
}

// TestPRsJumpToNeedingUser mirrors the same walk for api.PRNeedsUser: an approved PR (waiting on
// the merge) and an open PR in a project with no live reviewer both need the user; one in a project
// that HAS a live reviewer, and a merged one, do not — AnyLiveReviewer is scoped per project, so
// pr-reviewed and pr-unreviewed must sit in different ones.
func TestPRsJumpToNeedingUser(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.scopeRepo = 2, false
	m.w, m.h = 120, 40
	m.state = api.BoardState{
		Projects: []api.Project{{Tag: "reviewed-repo", Path: "/r/one"}, {Tag: "unreviewed-repo", Path: "/r/two"}},
		Agents:   []api.AgentView{{Name: "rev", Project: "reviewed-repo", Role: "reviewer", Status: "idle"}},
		PRs: []api.PR{
			{ID: "pr-reviewed", Status: "open", Project: "reviewed-repo", Agent: "a1", Branch: "b1", Base: "main"},
			{ID: "pr-approved", Status: "approved", Project: "reviewed-repo", Agent: "a2", Branch: "b2", Base: "main"},
			{ID: "pr-unreviewed", Status: "open", Project: "unreviewed-repo", Agent: "a3", Branch: "b3", Base: "main"},
			{ID: "pr-merged", Status: "merged", Project: "unreviewed-repo", Agent: "a4", Branch: "b4", Base: "main"},
		},
	}
	m.reclamp()

	needing := m.idsNeedingUser()
	if needing["pr-reviewed"] || !needing["pr-approved"] || !needing["pr-unreviewed"] || needing["pr-merged"] {
		t.Fatalf("idsNeedingUser = %+v, want exactly pr-approved and pr-unreviewed", needing)
	}

	m.selectRow("pr-reviewed")
	m.onKey("]")
	got1 := m.selID()
	m.onKey("]")
	got2 := m.selID()
	if got1 == got2 || !needing[got1] || !needing[got2] {
		t.Fatalf("] should visit both needing PRs in turn, got %q then %q", got1, got2)
	}
	m.onKey("]") // no third needing PR: no cycling
	if got := m.selID(); got != got2 {
		t.Errorf("] past the last needing PR moved to %q — it must not cycle", got)
	}
}

// TestMailJumpToNeedingUser mirrors the walk for mail: unread AND addressed to the user (today's
// inline check, sd-ac1757's predicate once it lands — never a second definition of it here).
// Messages to the user are grouped first by mailRows' own listing, so walking from a "rest" row
// (msg1, not to the user) must reach the to-you rows in EITHER direction, skipping the heading rows.
func TestMailJumpToNeedingUser(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.scopeRepo = 6, false
	m.w, m.h = 120, 40
	m.state = api.BoardState{
		Projects: []api.Project{{Tag: "repo", Path: "/r/one"}},
		Mail: []api.Mail{
			{ID: 1, Project: "repo", Agent: "nori", Sender: "hub", Body: "not to you"},
			{ID: 2, Project: "repo", Agent: api.SenderUser, Sender: "nori", Body: "to you, unread"},
			{ID: 3, Project: "repo", Agent: "nori", Sender: "hub", Body: "also not to you"},
			{ID: 4, Project: "repo", Agent: api.SenderUser, Sender: "dori", Body: "to you too, unread"},
		},
	}
	m.reclamp()
	m.selectRow(api.MailID(1))

	m.onKey("[") // above msg1: the to-you rows, nearest first
	if got := m.selID(); got != api.MailID(4) {
		t.Fatalf("[ from msg1 landed on %q, want %s", got, api.MailID(4))
	}
	m.onKey("[")
	if got := m.selID(); got != api.MailID(2) {
		t.Fatalf("[ again landed on %q, want %s", got, api.MailID(2))
	}
	m.onKey("[") // nothing to-you above msg2 (only the heading): no cycling
	if got := m.selID(); got != api.MailID(2) {
		t.Errorf("[ past the first to-you row moved to %q — it must not cycle", got)
	}

	m.selectRow(api.MailID(1))
	m.onKey("]") // below msg1: nothing to-you (they are all above it): no match at all
	if got := m.selID(); got != api.MailID(1) {
		t.Errorf("] below every to-you row should not move the cursor, got %q", got)
	}
	if m.flash == "" {
		t.Error("] with no later match should flash, not silently do nothing")
	}
}

// TestNeedsUserHasNoNotionOnSomeTabs: Repos, Chat and Runs have no such badge, so ]/[ do nothing —
// never advertised, per idsNeedingUser returning nil for them, and never moving a cursor.
func TestNeedsUserHasNoNotionOnSomeTabs(t *testing.T) {
	for _, tab := range []int{3, 4, 5} {
		m := newModel(nil, nil, "")
		m.tab = tab
		m.w, m.h = 120, 40
		if got := m.idsNeedingUser(); got != nil {
			t.Errorf("tab %d: idsNeedingUser = %v, want nil", tab, got)
		}
		before := m.cursor[tab]
		m.onKey("]")
		m.onKey("[")
		if m.cursor[tab] != before {
			t.Errorf("tab %d: ]/[ moved the cursor from %d to %d", tab, before, m.cursor[tab])
		}
	}
}

// TestNeedsYouFooterOnlyOnTabsWithTheNotion: a command that is not currently valid should not be
// offered, so [/] appears in the footer only for Tasks/Agents/PRs/Mail.
func TestNeedsYouFooterOnlyOnTabsWithTheNotion(t *testing.T) {
	with := map[keyScope]bool{
		scopeTasks: true, scopeAgents: true, scopePRs: true, scopeMail: true,
		scopeRepos: false, scopeChat: false, scopeRuns: false,
	}
	m := newModel(nil, nil, "")
	for scope, want := range with {
		got := strings.Contains(m.footerFor(scope), "[/]")
		if got != want {
			t.Errorf("scope %v: footer contains [/] = %v, want %v (footer: %q)", scope, got, want, m.footerFor(scope))
		}
	}
}

// TestNeedsYouJumpMovesTheListAndSyncsDetail is the review's blocker on the first submission: ]
// and [ returned nil before reaching the shared m.reclamp()/m.syncDetail() tail every other
// selection-moving key runs, so the list viewport never followed the jump (reclamp's
// m.list.SetCursor is what scrolls it into view) and the newly selected row's rich detail was never
// fetched (syncDetail is what updates m.detailKey and issues the fetch).
func TestNeedsYouJumpMovesTheListAndSyncsDetail(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 0
	m.w, m.h = 120, 24
	m.cl = &client.HTTP{} // syncDetail is a no-op with no client at all; give it one so detailKey moves
	var tasks []api.Task
	for i := 0; i < 60; i++ {
		tasks = append(tasks, api.Task{ID: fmt.Sprintf("t-%02d", i), Title: "filler", Status: "open", Approval: "approved", Priority: "P2"})
	}
	tasks[50].Approval = "pending" // the one needing task, far below the fold
	m.state = api.BoardState{Tasks: tasks}
	m.reclamp()
	m.cursor[0] = 0

	m.onKey("]")
	if got := m.selID(); got != "t-50" {
		t.Fatalf("precondition: ] should select t-50, got %q", got)
	}
	if m.list.Cursor != m.cursor[0] {
		t.Errorf("m.list.Cursor = %d, want %d — reclamp must follow the jump", m.list.Cursor, m.cursor[0])
	}
	if m.list.Cursor < m.list.Offset || m.list.Cursor >= m.list.Offset+m.list.Height {
		t.Errorf("the selected row (cursor %d) is outside the visible window [%d, %d)", m.list.Cursor, m.list.Offset, m.list.Offset+m.list.Height)
	}
	if want := fmt.Sprintf("%d:t-50", m.tab); m.detailKey != want {
		t.Errorf("m.detailKey = %q, want %q — the new selection's detail must be synced", m.detailKey, want)
	}
}

// TestCtrlLIsInertOnChat: Chat (tab 4) has no detail pane at all — actionableItems() excludes it in
// as many words — so ctrl+l must leave focus alone there, not enter focusDetail on a pane it does
// not render (review of sd-57e895's first submission: showDetail() is width-only and let it through).
func TestCtrlLIsInertOnChat(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 4
	m.w, m.h = 120, 40
	m.onKey("ctrl+l")
	if m.focus != focusList {
		t.Errorf("ctrl+l on Chat moved focus to %v, want it to stay inert", m.focus)
	}
	if m.flash == "" {
		t.Error("ctrl+l doing nothing on Chat should flash, not silently ignore the key")
	}
}

// TestCtrlLFlashesWithNoColumnToFocus: on a tab whose detail pane IS the right column (unlike
// PRs' diff), hiding that column leaves nothing for ctrl+l to reach — it must say so, not stay quiet.
func TestCtrlLFlashesWithNoColumnToFocus(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 0
	m.w, m.h = 120, 40
	m.hideDetail = true
	m.reclamp()
	m.onKey("ctrl+l")
	if m.focus != focusList {
		t.Errorf("ctrl+l with no column to focus moved focus to %v, want it to stay inert", m.focus)
	}
	if m.flash == "" {
		t.Error("ctrl+l with no column to focus should flash, not silently ignore the key")
	}
}

// TestBracketsNoLongerSwitchTabs pins requirement 2: ]/[ used to mirror tab/shift+tab, and this
// task retires that mapping. m.cursor comparisons alone would pass either way (a plain tab switch
// leaves the OLD tab's cursor untouched too), so this asserts m.tab itself is unchanged.
func TestBracketsNoLongerSwitchTabs(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 0
	m.w, m.h = 120, 40
	before := m.tab
	m.onKey("]")
	if m.tab != before {
		t.Errorf("] changed the tab from %d to %d — it must jump within the tab, not switch it", before, m.tab)
	}
	m.onKey("[")
	if m.tab != before {
		t.Errorf("[ changed the tab from %d to %d — it must jump within the tab, not switch it", before, m.tab)
	}
}

// TestFooterRowsStayWithinBudget pins each tab's context-row length as of this task's needs-you
// addition: the two footer rows are a fixed, scarce resource (the parent epic sd-a6e884 names
// them), and any silent growth here is a silent truncation of "space actions" — the entry
// footerFor always appends last — at whatever width used to show the row in full (review round 3).
// Bumping a ceiling is a deliberate budget call a future change should make on purpose.
func TestFooterRowsStayWithinBudget(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	for _, tc := range []struct {
		scope keyScope
		max   int
	}{
		{scopeTasks, 170},
		{scopeAgents, 150},
		{scopePRs, 135},
		{scopeMail, 110},
	} {
		if got := len(m.footerFor(tc.scope)); got > tc.max {
			t.Errorf("scope %d context row is %d chars, budget is %d — growth here silently truncates the row at widths that used to show it whole", tc.scope, got, tc.max)
		}
	}
}
