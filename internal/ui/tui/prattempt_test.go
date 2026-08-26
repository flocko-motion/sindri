package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// TestTheAttemptColumnSpeaksOnlyWhenItMatters: 66 of this fleet's PRs landed first time and one took
// twelve, and the list showed both identically — so a reader could not tell a review doing its job
// from an author stuck in a submit loop. A first attempt stays BLANK: a column repeating "×1" on
// most rows is noise, and what wants spotting is the row on its fourth.
func TestTheAttemptColumnSpeaksOnlyWhenItMatters(t *testing.T) {
	for _, tc := range []struct {
		attempt int
		want    string
	}{
		{0, ""}, // an older hub sends nothing; say nothing rather than claim a first attempt
		{1, ""},
		{2, "×2"},
		{12, "×12"},
	} {
		if got := theme.AttemptCell(tc.attempt); got != tc.want {
			t.Errorf("AttemptCell(%d) = %q, want %q", tc.attempt, got, tc.want)
		}
	}
}

// TestTheAttemptReachesTheRow: the count is the hub's to derive (from the PR's own events) and the
// front-end's only to render — the same division Reviewer and Approvals already follow.
func TestTheAttemptReachesTheRow(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab, m.w, m.h = 2, 120, 40
	m.state = api.BoardState{
		Projects: []api.Project{{Tag: "p", Path: "/r/one"}},
		PRs: []api.PR{
			{ID: "pr-sd-1", Project: "p", Status: "open", Agent: "austri", Branch: "sd-1", Attempt: 1},
			{ID: "pr-sd-9", Project: "p", Status: "rejected", Agent: "bombur", Branch: "sd-9", Attempt: 4},
		},
	}
	var first, fourth string
	for _, r := range m.prRows() { // a header row rides along, so find the PRs by their ids
		switch {
		case strings.Contains(r.text, "pr-sd-1 "):
			first = r.text
		case strings.Contains(r.text, "pr-sd-9 "):
			fourth = r.text
		}
	}
	if first == "" || fourth == "" {
		t.Fatalf("both PRs should have rows: %q / %q", first, fourth)
	}
	if strings.Contains(first, "×") {
		t.Errorf("a first attempt should carry no mark: %q", first)
	}
	if !strings.Contains(fourth, "×4") {
		t.Errorf("a fourth attempt should say so: %q", fourth)
	}
}
