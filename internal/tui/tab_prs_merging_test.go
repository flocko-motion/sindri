package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

// prRowText returns the rendered PRs-tab row for a PR id (empty if absent).
func prRowText(m model, id string) string {
	for _, r := range m.prRows() {
		if r.id == id {
			return r.text
		}
	}
	return ""
}

// TestMergingTransient: triggering a merge shows a transient "merging" on the row
// at once (before the hub confirms), which is replaced by the real status when a
// fresh board snapshot lands, and cleared on error — the immediate-feedback path.
func TestMergingTransient(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 2 // PRs
	m.state = hub.BoardState{PRs: []store.PR{{ID: "pr-td-1", Status: "approved", Project: "repo", Agent: "brokkr", Branch: "td-1"}}}

	// Baseline: the row shows the real status, not "merging".
	if txt := prRowText(m, "pr-td-1"); !strings.Contains(txt, "approved") || strings.Contains(txt, "merging") {
		t.Fatalf("baseline row = %q, want approved and not merging", txt)
	}

	// Trigger: optimistic "merging" shows immediately, before any hub round-trip.
	m.markMerging("pr-td-1")
	if txt := prRowText(m, "pr-td-1"); !strings.Contains(txt, "merging") {
		t.Fatalf("after trigger row = %q, want merging", txt)
	}

	// Confirm: a fresh snapshot showing it merged clears the transient (reconcile),
	// and the row shows the real "merged".
	m.state = hub.BoardState{PRs: []store.PR{{ID: "pr-td-1", Status: "merged", Project: "repo"}}}
	m.reconcileMerging()
	if m.merging["pr-td-1"] {
		t.Fatalf("marker should clear once the board confirms merged")
	}
	if txt := prRowText(m, "pr-td-1"); !strings.Contains(txt, "merged") || strings.Contains(txt, "merging") {
		t.Fatalf("after confirm row = %q, want merged and not merging", txt)
	}
}

// TestMergeDoneClearsTransient: the mergeDoneMsg handler clears the marker on both
// success (applying the fresh state) and failure (surfacing the error modal).
func TestMergeDoneClearsTransient(t *testing.T) {
	// Success.
	m := newModel(nil, nil, "")
	m.markMerging("pr-td-1")
	merged := hub.BoardState{PRs: []store.PR{{ID: "pr-td-1", Status: "merged", Project: "repo"}}}
	tm, _ := m.Update(mergeDoneMsg{id: "pr-td-1", state: merged})
	got := tm.(model)
	if got.merging["pr-td-1"] {
		t.Fatalf("success should clear the merging marker")
	}
	if len(got.state.PRs) != 1 || got.state.PRs[0].Status != "merged" {
		t.Fatalf("success should apply the fresh state: %+v", got.state.PRs)
	}

	// Failure: marker cleared, error surfaced, state untouched.
	m2 := newModel(nil, nil, "")
	m2.markMerging("pr-td-1")
	tm2, _ := m2.Update(mergeDoneMsg{id: "pr-td-1", err: fmt.Errorf("merge conflict")})
	got2 := tm2.(model)
	if got2.merging["pr-td-1"] {
		t.Fatalf("failure should clear the merging marker")
	}
	if got2.errText != "merge conflict" {
		t.Fatalf("failure should surface the error modal, got %q", got2.errText)
	}
}
