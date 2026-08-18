package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/api"
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

// TestActiveDefaultShowsARecentlyMergedPR is the behaviour change sd-2aac4e calls out
// explicitly: unlike the old "unmerged" default, a PR merged moments ago stays visible under the
// new default (active) — merged is not by itself a reason to hide it.
func TestActiveDefaultShowsARecentlyMergedPR(t *testing.T) {
	m := newModel(nil, nil, "")
	m.scopeRepo = false
	if m.prFilter != api.PRFilterActive {
		t.Fatalf("prFilter default = %q, want active", m.prFilter)
	}
	m.state = api.BoardState{PRs: []api.PR{
		{ID: "pr-recent", Status: "merged", Project: "repo", UpdatedAt: time.Now().UTC().Format(time.RFC3339)},
	}}
	if txt := prRowText(m, "pr-recent"); !strings.Contains(txt, "merged") {
		t.Fatalf("a just-merged PR should still show under the active default, got %q", txt)
	}
}

// TestMergingTransient: triggering a merge shows a transient "merging" on the row
// at once (before the hub confirms), which is replaced by the real status when a
// fresh board snapshot lands, and cleared on error — the immediate-feedback path.
func TestMergingTransient(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 2           // PRs
	m.scopeRepo = false // global scope: this test is about merging state, not repo filtering (which now defaults on)
	m.state = api.BoardState{PRs: []api.PR{{ID: "pr-td-1", Status: "approved", Project: "repo", Agent: "brokkr", Branch: "td-1"}}}

	// Baseline: the row shows the real status, not "merging".
	if txt := prRowText(m, "pr-td-1"); !strings.Contains(txt, "approved") || strings.Contains(txt, "merging") {
		t.Fatalf("baseline row = %q, want approved and not merging", txt)
	}

	// Trigger: optimistic "merging" shows immediately, before any hub round-trip.
	m.markMerging("pr-td-1")
	if txt := prRowText(m, "pr-td-1"); !strings.Contains(txt, "merging") {
		t.Fatalf("after trigger row = %q, want merging", txt)
	}

	// Confirm: a fresh snapshot showing it merged clears the transient (reconcile).
	m.state = api.BoardState{PRs: []api.PR{{ID: "pr-td-1", Status: "merged", Project: "repo"}}}
	m.reconcileMerging()
	if m.merging["pr-td-1"] {
		t.Fatalf("marker should clear once the board confirms merged")
	}
	// A merged PR with no recorded change time is hidden by the active default (unrecognized
	// as recent), so the row drops out of the list.
	if txt := prRowText(m, "pr-td-1"); txt != "" {
		t.Fatalf("merged PR with no update time should be hidden by the active default, got %q", txt)
	}
	// With the filter showing everything, the row reappears with the real "merged" status
	// (and no lingering transient "merging").
	m.prFilter = api.PRFilterAll
	if txt := prRowText(m, "pr-td-1"); !strings.Contains(txt, "merged") || strings.Contains(txt, "merging") {
		t.Fatalf("with filter=all, row = %q, want merged and not merging", txt)
	}
}

// TestMergeDoneClearsTransient: the mergeDoneMsg handler clears the marker on both
// success (applying the fresh state) and failure (surfacing the error modal).
func TestMergeDoneClearsTransient(t *testing.T) {
	// Success.
	m := newModel(nil, nil, "")
	m.markMerging("pr-td-1")
	merged := api.BoardState{PRs: []api.PR{{ID: "pr-td-1", Status: "merged", Project: "repo"}}}
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
