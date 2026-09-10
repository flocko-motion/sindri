package store

import "testing"

// TestAnyTaskTypeTakesPartInTheHierarchy is the point of holding parentage here: neither an openspec
// change nor a GitHub issue carries the notion upstream, so a link either lives in sindri's own
// representation or cannot exist at all. Both directions have to work.
func TestAnyTaskTypeTakesPartInTheHierarchy(t *testing.T) {
	ps := ownedStore(t)
	// A task sindri owns, parenting an openspec change and a GitHub issue.
	for _, child := range []string{"os-a1b2c3", "gh-42"} {
		if err := ps.SetParent(child, "td-abc123"); err != nil {
			t.Fatalf("parent %s: %v", child, err)
		}
		if got := ps.ParentOf(child); got != "td-abc123" {
			t.Errorf("parent of %s = %q, want td-abc123", child, got)
		}
	}
	// And the reverse: an openspec change as the parent of a task sindri owns.
	if err := ps.SetParent("td-child1", "os-a1b2c3"); err != nil {
		t.Fatalf("parent under an openspec change: %v", err)
	}
	if got := ps.ParentOf("td-child1"); got != "os-a1b2c3" {
		t.Errorf("parent of td-child1 = %q, want os-a1b2c3", got)
	}
}

// TestParentLinksAreWhatTheSyncApplies: the read model is rebuilt from the sources, so the links have
// to survive as a set the sync can lay back over it.
func TestParentLinksAreWhatTheSyncApplies(t *testing.T) {
	ps := ownedStore(t)
	for child, parent := range map[string]string{"td-1": "td-root", "os-9": "td-root", "gh-7": "os-9"} {
		if err := ps.SetParent(child, parent); err != nil {
			t.Fatal(err)
		}
	}
	links, err := ps.ParentLinks()
	if err != nil {
		t.Fatalf("ParentLinks: %v", err)
	}
	if len(links) != 3 {
		t.Fatalf("want 3 links, got %d: %v", len(links), links)
	}
	if links["gh-7"] != "os-9" {
		t.Errorf("a GitHub issue under an openspec change should survive, got %q", links["gh-7"])
	}
}

// TestReParentingReplacesTheLink: one parent per task, so a second write moves it rather than adding.
func TestReParentingReplacesTheLink(t *testing.T) {
	ps := ownedStore(t)
	if err := ps.SetParent("td-1", "td-first"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetParent("td-1", "td-second"); err != nil {
		t.Fatal(err)
	}
	if got := ps.ParentOf("td-1"); got != "td-second" {
		t.Errorf("parent = %q, want td-second", got)
	}
	links, _ := ps.ParentLinks()
	if len(links) != 1 {
		t.Errorf("re-parenting must replace, not accumulate: %v", links)
	}
}

// TestClearingAParentMakesItARoot, which an empty parent in an edit has to mean.
func TestClearingAParentMakesItARoot(t *testing.T) {
	ps := ownedStore(t)
	if err := ps.SetParent("td-1", "td-root"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetParent("td-1", ""); err != nil {
		t.Fatalf("clearing via an empty parent: %v", err)
	}
	if got := ps.ParentOf("td-1"); got != "" {
		t.Errorf("parent = %q, want none", got)
	}
}

// TestParentageIsProjectScoped: two repos may hold the same id, and one's hierarchy must not shape
// the other's.
func TestParentageIsProjectScoped(t *testing.T) {
	st := ownedStore(t).s
	if err := st.RegisterProject("other", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := st.For("proj").SetParent("td-1", "td-root"); err != nil {
		t.Fatal(err)
	}
	if got := st.For("other").ParentOf("td-1"); got != "" {
		t.Errorf("project other sees %q as the parent of td-1", got)
	}
}
