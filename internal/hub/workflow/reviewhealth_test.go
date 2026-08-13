package workflow

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

func TestNeedsReview(t *testing.T) {
	for _, tc := range []struct {
		name string
		pr   store.PR
		live map[string]bool
		want bool
	}{
		{"open, no row", store.PR{ID: "pr-a", Status: "open"}, map[string]bool{}, true},
		{"open, live row", store.PR{ID: "pr-a", Status: "open"}, map[string]bool{"pr-a": true}, false},
		{"interim, no row", store.PR{ID: "pr-a", Status: "open", Kind: "interim"}, map[string]bool{}, false},
		{"not open", store.PR{ID: "pr-a", Status: "approved"}, map[string]bool{}, false},
	} {
		if got := needsReview(tc.pr, tc.live); got != tc.want {
			t.Errorf("%s: needsReview = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestRepairReviewRowsFindsAGap covers the backstop: an open final PR with no live review row —
// however it got that way — gets one, while an interim PR and one already covered are left alone.
func TestRepairReviewRowsFindsAGap(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	for _, p := range []store.PR{
		{ID: "pr-gap", Task: "td-gap", Agent: "bombur", Branch: "pr-gap", Base: "main", Status: "open"},
		{ID: "pr-covered", Task: "td-covered", Agent: "bombur", Branch: "pr-covered", Base: "main", Status: "open"},
		{ID: "pr-milestone", Task: "td-mile", Agent: "bombur", Branch: "pr-milestone", Base: "main", Status: "open", Kind: "interim"},
	} {
		if err := ps.PutPR(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ps.AddReview("pr-covered", "already in hand"); err != nil {
		t.Fatal(err)
	}

	e := New(st, &stubDeps{root: t.TempDir(), alive: true})
	e.RepairReviewRows("repo")

	gapRevs, _ := ps.Reviews("pr-gap")
	if len(gapRevs) == 0 {
		t.Error("pr-gap should have gained a review row")
	}
	coveredRevs, _ := ps.Reviews("pr-covered")
	if len(coveredRevs) != 1 {
		t.Errorf("pr-covered should keep its single row, got %d", len(coveredRevs))
	}
	milestoneRevs, _ := ps.Reviews("pr-milestone")
	if len(milestoneRevs) != 0 {
		t.Error("an interim (milestone/contribution) PR must never gain a review row")
	}
}
