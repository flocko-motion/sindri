package store

import "testing"

// TestCommentsReconcilePerSource: ReplaceComments swaps a source's whole set (so a
// re-sync adds new, drops removed, updates changed) without touching other sources.
func TestCommentsReconcilePerSource(t *testing.T) {
	p := openTmpProject(t)

	// First github sync: two comments.
	if err := p.ReplaceComments("gh-1", "github", []Comment{
		{Source: "github", SourceRef: "url/a", Author: "ann", Body: "first", CreatedAt: "2026-01-01T00:00:00Z"},
		{Source: "github", SourceRef: "url/b", Author: "bob", Body: "second", CreatedAt: "2026-01-02T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	// A td comment on the same task, a different source.
	if err := p.ReplaceComments("gh-1", "td", []Comment{
		{Source: "td", SourceRef: "c1", Author: "sess", Body: "td note", CreatedAt: "2026-01-03T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	// Re-sync github: b removed, a edited, c added — td must be untouched.
	if err := p.ReplaceComments("gh-1", "github", []Comment{
		{Source: "github", SourceRef: "url/a", Author: "ann", Body: "first (edited)", CreatedAt: "2026-01-01T00:00:00Z"},
		{Source: "github", SourceRef: "url/c", Author: "cid", Body: "third", CreatedAt: "2026-01-04T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := p.Comments("gh-1")
	if err != nil {
		t.Fatal(err)
	}
	// Ordered by created_at: a(edited), td note, c — b is gone.
	if len(got) != 3 {
		t.Fatalf("want 3 comments (a, td, c), got %d: %+v", len(got), got)
	}
	if got[0].SourceRef != "url/a" || got[0].Body != "first (edited)" {
		t.Errorf("edited comment not reconciled: %+v", got[0])
	}
	if got[1].Source != "td" || got[1].Body != "td note" {
		t.Errorf("td comment (other source) should survive a github re-sync: %+v", got[1])
	}
	if got[2].SourceRef != "url/c" {
		t.Errorf("new comment not added: %+v", got[2])
	}
	for _, c := range got {
		if c.SourceRef == "url/b" {
			t.Error("removed comment url/b should be gone after re-sync")
		}
	}
}
