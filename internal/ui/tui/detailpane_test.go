package tui

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// detailPaneFixture is one board with enough cross-references wired for every kind of item a
// detail pane can offer: a task with both a parent and a child, an agent holding it with a PR,
// mail between two real agents (so the traceable "from:" xref has something to point at), and a
// run tied to both an agent and a task. The parent is closed and the agent's own PR is merged —
// both hidden by the Tasks/PRs tabs' own default filters — so a jump to either exercises sd-15f9a1's
// "arrive narrowed to what the item named," not just the case where the target was visible anyway.
func detailPaneFixture() api.BoardState {
	return api.BoardState{
		Projects: []api.Project{{Tag: "here", Path: "/r/here"}},
		Agents: []api.AgentView{
			{Project: "here", Repo: "here", Name: "nori", Role: "worker", Status: "idle", Task: "sd-child", PR: "pr-old", Workspace: "agents/nori"},
			{Project: "here", Repo: "here", Name: "dwalin", Role: "worker", Status: "idle"},
		},
		Tasks: []api.Task{
			{ID: "sd-parent", Title: "parent", Status: "closed"},
			{ID: "sd-child", Title: "child", ParentID: "sd-parent"},
		},
		PRs: []api.PR{
			{ID: "pr-1", Agent: "nori", Task: "sd-child", Project: "here", Status: "open", Branch: "b1", Base: "main"},
			{ID: "pr-old", Agent: "nori", Project: "here", Status: "merged", Branch: "old", Base: "main"},
		},
		Runs: []api.Run{
			{ID: "run-1", Project: "here", Agent: "nori", Task: "sd-child", Command: "go test", Status: "queued", CreatedAt: "2026-08-18T00:00:00Z"},
		},
		Mail: []api.Mail{
			{ID: 1, Project: "here", Agent: "nori", Sender: "dwalin", Body: "hi", SentAt: "2026-08-18T00:00:00Z"},
		},
	}
}

// noDetailPane names tabs that genuinely have no detail column to focus — the argument for each
// exception goes on record here rather than being assumed (parity_test.go's oneSided is the same
// shape, for the same reason).
var noDetailPane = map[int]string{
	4: "Chat renders its own full-width transcript; there is no right-hand detail column at all",
}

// detailPaneModel is one tab, wide enough to show its detail column, with the fixture's row for
// that tab selected — and, where a tab's detail is fetched lazily, that fetch already landed.
func detailPaneModel(tab int) model {
	m := newModel(nil, nil, "/r/here")
	m.tab, m.w, m.h = tab, 120, 40
	m.state = detailPaneFixture()
	m.reclamp()
	switch tab {
	case 0:
		m.selectRow("sd-child")
	case 1:
		m.selectRow("nori")
	case 2:
		m.selectRow("pr-1")
		m.prDetail = api.PRDetail{PR: m.state.PRs[0], Task: api.Task{ID: "sd-child", Title: "child"}}
	case 3:
		m.selectRow("here")
	case 5:
		m.selectRow("run-1")
		m.runDetail = api.RunDetail{Run: m.state.Runs[0]}
	case 6:
		m.selectRow(api.MailID(1))
	}
	return m
}

// TestEveryDetailPaneIsActionable derives its coverage from the tab list rather than naming the
// tabs that happen to be wired today (sd-15f9a1): a tab added later, or one that regresses to an
// empty actionableItems(), fails here instead of just going quiet.
func TestEveryDetailPaneIsActionable(t *testing.T) {
	for tab := range tuiSections {
		if reason, skip := noDetailPane[tab]; skip {
			t.Logf("tab %d (%s) has no detail pane: %s", tab, tuiSections[tab].Title, reason)
			continue
		}
		t.Run(tuiSections[tab].Key, func(t *testing.T) {
			m := detailPaneModel(tab)
			if !m.showDetail() {
				t.Fatal("precondition: the fixture terminal should be wide enough to show the detail pane")
			}
			act := m.actionableItems()
			if len(act) == 0 {
				t.Fatalf("tab %d (%s) offers a detail pane but nothing in it is focusable", tab, tuiSections[tab].Title)
			}

			// Focus: ctrl+l reaches the pane whenever it has something to focus.
			m.onKey("ctrl+l")
			if !m.rightFocus {
				t.Fatal("ctrl+l did not focus a detail pane with actionable items")
			}

			// Movement: j walks the cursor forward without ever leaving the actionable set.
			for range act {
				m.onKey("j")
				if m.rightCursor < 0 || m.rightCursor >= len(act) {
					t.Fatalf("j moved the cursor out of range: %d (of %d)", m.rightCursor, len(act))
				}
			}

			// Yank: the focused item's value is what lands in the flash — the same thing every other
			// yank test in this package checks (clipboard.WriteAll's own success is not this test's
			// business).
			it, ok := m.focusedItem()
			if !ok {
				t.Fatal("the cursor should be on a real item after walking through all of them")
			}
			m.onKey("y")
			if m.flash != "copied: "+it.value {
				t.Errorf("y should yank the focused value, got flash %q for value %q", m.flash, it.value)
			}

			// Jump: `g` on a cross-reference (task/agent/pr) is the actual navigation (items.go's
			// own convention: "g goes to where the item lives"), and it must land selecting exactly
			// the row it named — never a stray row on the tab it switched to, and never a no-op that
			// looks like the key does nothing — even when the destination's own filter hides the
			// target (sd-15f9a1: "a destination must arrive narrowed to what the item named"). enter
			// instead peeks the item's details in a modal without moving; mail is the one kind enter
			// itself navigates for (it narrows a set rather than selecting one row). Checked for
			// every actionable item, each on its own freshly built model since a jump mutates the
			// tab and possibly the filter under it.
			for idx, item := range act {
				switch item.kind {
				case "task", "agent", "pr":
					jumped := detailPaneModel(tab)
					jumped.rightFocus, jumped.rightCursor = true, idx
					jumped.onKey("g")
					if got := jumped.selID(); got != item.value {
						t.Errorf("g on %s %q should land selecting it, got %q on tab %d",
							item.kind, item.value, got, jumped.tab)
					}

					peeked := detailPaneModel(tab)
					peeked.rightFocus, peeked.rightCursor = true, idx
					peeked.onKey("enter")
					if !peeked.modal {
						t.Errorf("enter on %s %q should open its details modal", item.kind, item.value)
					}
				case "mail":
					jumped := detailPaneModel(tab)
					jumped.rightFocus, jumped.rightCursor = true, idx
					jumped.onKey("enter")
					if jumped.tab != 6 {
						t.Errorf("enter on a mail item should land on the Mail tab, got tab %d", jumped.tab)
					}
				case "view", "path", "url", "resume":
					// Each already has its own dedicated test (agent view toggle, url copy,
					// resume choice) — nothing further to assert here.
				default:
					t.Errorf("item kind %q is not handled by this test — add a case for it", item.kind)
				}
			}
		})
	}
}
