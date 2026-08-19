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

// TestSignedOutOutranksEveryPhase: eitri sat signed out for hours reading "idle" on the board,
// indistinguishable from a planner at rest, while hub messages piled into an input box that could
// not send them. Whatever the hub last asked of an agent, a signed-out one is not doing it — and
// unlike every other status here, it cannot even be told so.
func TestSignedOutOutranksEveryPhase(t *testing.T) {
	for _, status := range []string{"idle", "working", "planning", "collab", "reviewing", "submitted", "stalled"} {
		if got := overlayRuntime(status, "signed-out"); got != "signed-out" {
			t.Errorf("overlayRuntime(%q, signed-out) = %q, want signed-out", status, got)
		}
	}
	// A phase the runtime says nothing about is still the phase: a failed probe reports "".
	if got := overlayRuntime("planning", ""); got != "planning" {
		t.Errorf("a silent probe must change nothing, got %q", got)
	}
}

// TestRuntimeDecidesIdleOrWorking: eitri was planning — the pane showed the interrupt hint, 1m41s
// and 5.1k tokens into a turn — while the board read "idle", because a planner holds no task, feature
// or PR by design and a stale guard discarded "working" against that empty column. Whether an agent
// is idle is the RUNTIME's answer, an observed fact about the pane; what it holds is a separate
// column the board already shows, and "working, holding nothing" is a coherent, honest state rather
// than a contradiction to paper over — the ordinary state for a planner, and a real one worth seeing
// for any other role too.
func TestRuntimeDecidesIdleOrWorking(t *testing.T) {
	if got := overlayRuntime("idle", "working"); got != "working" {
		t.Errorf("a moving pane is working even holding nothing, got %q", got)
	}
	if got := overlayRuntime("working", "idle"); got != "idle" {
		t.Errorf("a still pane is idle even if the phase says working, got %q", got)
	}
	// The states that need a human are about the agent, not its workload, so they still outrank.
	for _, rt := range []string{"blocked", "signed-out", "api-error"} {
		if got := overlayRuntime("idle", rt); got != rt {
			t.Errorf("%s must show regardless of what is held, got %q", rt, got)
		}
	}
	// A specific phase survives a runtime that only disagrees on the generic idle/working ambiguity —
	// overlayRuntime replaces the generic words, never a more meaningful one.
	if got := overlayRuntime("planning", "working"); got != "planning" {
		t.Errorf("a specific phase should not be flattened to the generic runtime word, got %q", got)
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
		Agents: []api.AgentView{{Name: "dvalin", Status: api.StatusBlocked}, {Name: "fili", Status: "idle"}},
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
