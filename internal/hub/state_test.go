package hub

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestAgentPRMatchesThePRsTab: the PR the Agents tab shows against an author is one the PRs tab
// lists, so the two tabs never disagree about whether that agent has a PR at all. A scrapped one
// used to stick to its author on the Agents tab and be absent from the PRs tab.
func TestAgentPRMatchesThePRsTab(t *testing.T) {
	prs := []api.PR{
		{ID: "pr-1", Project: "p", Agent: "bombur", Status: "scrapped"},
		{ID: "pr-2", Project: "p", Agent: "dain", Status: "merged"},
		{ID: "pr-3", Project: "p", Agent: "jari", Status: "open"},
		{ID: "pr-4", Project: "p", Agent: "nori", Status: "rejected"},
	}
	for agent, want := range map[string]string{"bombur": "", "dain": "", "jari": "pr-3", "nori": "pr-4"} {
		if got := openPRFor(prs, "p", agent); got != want {
			t.Errorf("openPRFor(%s) = %q, want %q", agent, got, want)
		}
	}
}

// TestOrphanScanReadsTheWatchdogsListing: the orphan scan reads the sweep's own last listing
// (-> hub/watchdog.go's pods) rather than taking a second one of its own — a stopped watchdog, so
// nothing in the background can overwrite it mid-test (-> boardfill_test.go's stillWatchdog).
func TestOrphanScanReadsTheWatchdogsListing(t *testing.T) {
	h := newHub(t)
	w := stillWatchdog(t, h)
	w.mu.Lock()
	w.listing = []string{"sindri-proj-nobody"}
	w.mu.Unlock()

	board, err := h.State("")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range board.Orphans {
		if o == "sindri-proj-nobody" {
			found = true
		}
	}
	if !found {
		t.Errorf("a pod in the watchdog's listing with no roster entry should read as an orphan, got %v", board.Orphans)
	}
}

// TestBoardCarriesItsSections: the tabs and their markers travel WITH the board, because a
// front-end may not link the hub and so cannot resolve them for itself. A board served without
// them would render a fleet of blocked agents as an ordinary one.
func TestBoardCarriesItsSections(t *testing.T) {
	b := withSections(BoardState{
		Tasks:  []api.Task{{ID: "a", Status: "open", Approval: "pending"}},
		Agents: []api.AgentView{{Name: "dvalin", Status: api.StatusBlocked, NeedsUser: true}, {Name: "fili", Status: "idle"}},
	})
	if len(b.Sections) == 0 {
		t.Fatal("the board must carry the sections the UIs draw")
	}
	for key, want := range map[string]int{"tasks": 1, "agents": 1, "prs": 0} {
		if got := b.SectionAttention(key); got != want {
			t.Errorf("%s attention = %d, want %d", key, got, want)
		}
	}
}

// TestAnUnreachableAgentWearsTheWordAndNeedsTheUser: an agent nothing can be said to is holding work
// nobody can redirect, and it reads "idle" while the messages vanish. The word is its own, beside the
// two it was mistaken for — and it must count against the badge, or nothing on the board points at it.
func TestAnUnreachableAgentWearsTheWordAndNeedsTheUser(t *testing.T) {
	// Over what an agent is doing, and over the words for waiting: none of them can be told anything.
	for _, was := range []string{"idle", "working", "submitted", api.StatusStalled, api.StatusEscalated} {
		if got := overlayUnreachable(was, true); got != api.StatusUnreachable {
			t.Errorf("overlayUnreachable(%q) = %q, want %q", was, got, api.StatusUnreachable)
		}
	}
	// Three outrank it, each naming a remedy where this word names none — blocked among them, whose
	// remedy IS a message: its dialog consumes keystrokes without drawing them, so a stale count there
	// would replace "answer it" with "nothing reaches it" on an agent one keypress fixes.
	// "" among them: an agent the sweep has not reached yet supports no claim, and AgentNotUp says so.
	for _, was := range []string{"", "down", "stopped", api.StatusUnknown, "launching", api.StatusSignedOut, api.StatusBlocked} {
		if got := overlayUnreachable(was, true); got != was {
			t.Errorf("overlayUnreachable(%q) = %q, want it unchanged", was, got)
		}
	}
	if got := overlayUnreachable("working", false); got != "working" {
		t.Errorf("an agent whose messages land must be untouched, got %q", got)
	}
	if !api.AgentNeedsUser(api.AgentView{Status: api.StatusUnreachable}) {
		t.Error("an unreachable agent must count as needing the user")
	}
}
