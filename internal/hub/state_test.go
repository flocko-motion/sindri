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
		if got := overlayRuntime(status, "signed-out", true); got != "signed-out" {
			t.Errorf("overlayRuntime(%q, signed-out) = %q, want signed-out", status, got)
		}
	}
	// A phase the runtime says nothing about is still the phase: a failed probe reports "".
	if got := overlayRuntime("planning", "", true); got != "planning" {
		t.Errorf("a silent probe must change nothing, got %q", got)
	}
}

// TestAMovingPaneIsNotWorkInHand: bombur read "working" against an empty task column. It was
// reacting to a "there may be new work" broadcast — its screen moved, which is all the runtime can
// see, but a worker holding nothing cannot be working. Activity decides the runtime word; whether
// that word may claim the status is the workflow's to say.
func TestAMovingPaneIsNotWorkInHand(t *testing.T) {
	if got := overlayRuntime("idle", "working", false); got != "idle" {
		t.Errorf("an agent holding nothing must not read as working, got %q", got)
	}
	if got := overlayRuntime("idle", "working", true); got != "working" {
		t.Errorf("holding work, a moving pane IS working, got %q", got)
	}
	// The states that need a human are about the agent, not its workload, so they still apply.
	for _, rt := range []string{"blocked", "signed-out"} {
		if got := overlayRuntime("idle", rt, false); got != rt {
			t.Errorf("%s must show even with nothing held, got %q", rt, got)
		}
	}
	// And going quiet still reads idle either way — that claims nothing that could be untrue.
	if got := overlayRuntime("working", "idle", false); got != "idle" {
		t.Errorf("a still pane is idle, got %q", got)
	}
}

// TestFullnessOnlyExplainsAnIdleAgent: "full" is a REASON an idle agent is passed over, not an
// activity. Applied unconditionally it overwrote the one fact the status column carries, so an
// agent mid-task read "full" — inviting the user to clear a context the hub refuses to clear at a
// leaf boundary, which is how they learned the board was lying.
func TestFullnessOnlyExplainsAnIdleAgent(t *testing.T) {
	for _, tc := range []struct {
		name              string
		status            string
		task, feature, pr string
		want              string
	}{
		{"idle and holding nothing is the one case", "idle", "", "", "", "full"},
		{"working keeps working", "working", "sd-1", "", "", "working"},
		{"blocked keeps blocked — it needs the user now", "blocked", "sd-1", "", "", "blocked"},
		{"submitted keeps submitted", "submitted", "sd-1", "", "pr-1", "submitted"},
		{"down keeps down", "down", "", "", "", "down"},
		// Stalled sits directly above fullness and has the same shape. A stalled agent HOLDS work,
		// and stalled is the more actionable word, so it must survive.
		{"stalled keeps stalled", "stalled", "sd-1", "", "", "stalled"},
		// The reason held work is checked directly: a quiet runtime probe reads a task-holder as
		// idle, and that agent is not idle in the sense fullness explains.
		{"idle but holding a task", "idle", "sd-1", "", "", "idle"},
		{"idle but holding a feature", "idle", "", "feat-1", "", "idle"},
		{"idle but carrying a PR", "idle", "", "", "pr-1", "idle"},
	} {
		if got := overlayFullness(tc.status, true, tc.task, tc.feature, tc.pr); got != tc.want {
			t.Errorf("%s: overlayFullness(%q) = %q, want %q", tc.name, tc.status, got, tc.want)
		}
	}
}

// TestNotFullChangesNothing: the overlay is only ever additive, so a fleet under the threshold
// reads exactly as it did before.
func TestNotFullChangesNothing(t *testing.T) {
	for _, status := range []string{"idle", "working", "blocked", "submitted", "stalled", "down"} {
		if got := overlayFullness(status, false, "", "", ""); got != status {
			t.Errorf("overlayFullness(%q, full=false) = %q, want it unchanged", status, got)
		}
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
