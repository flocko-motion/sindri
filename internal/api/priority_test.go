package api

import "testing"

// priorityTree is one parent with a rated child, an unrated grandchild, an unrated child and a
// closed one — the shapes a scoped rating has to tell apart.
func priorityTree(parentStatus string) []Task {
	return []Task{
		{ID: "td-epic", Status: parentStatus, Priority: "P1"},
		{ID: "td-a", Status: "open", ParentID: "td-epic", Priority: "P0"},
		{ID: "td-deep", Status: "open", ParentID: "td-a"},
		{ID: "td-b", Status: "open", ParentID: "td-epic"},
		{ID: "td-done", Status: "closed", ParentID: "td-epic"},
		{ID: "td-other", Status: "open"}, // outside the tree
	}
}

// TestPriorityEffectCountsOnlyOpenTasksBelow: the counts are what a menu promises to change, so a
// closed descendant must not inflate them and a task outside the tree must not appear at all.
func TestPriorityEffectCountsOnlyOpenTasksBelow(t *testing.T) {
	got := PriorityEffect(priorityTree("open"), "td-epic")
	if got.Children != 3 { // td-a, td-deep, td-b — never td-done or td-other
		t.Errorf("Children = %d, want 3 (the open descendants at any depth)", got.Children)
	}
	if got.Unrated != 2 { // td-deep and td-b; td-a is deliberately rated
		t.Errorf("Unrated = %d, want 2", got.Unrated)
	}
	if leaf := PriorityEffect(priorityTree("open"), "td-b"); leaf.Children != 0 {
		t.Errorf("a leaf has nothing below it, got %d", leaf.Children)
	}
}

// TestIndependentIsWhetherTheParentHasEnded is the distinction the whole scope offer rests on. While
// the parent is open its children are claimed as one package and a rating only orders them; once it
// has ended each child stands alone and a rating is what makes it claimable. A front-end that got
// this backwards would tell a user it had released work it had merely reordered.
func TestIndependentIsWhetherTheParentHasEnded(t *testing.T) {
	if PriorityEffect(priorityTree("open"), "td-epic").Independent {
		t.Error("an OPEN parent is claimed as a package — its children are not claimed independently")
	}
	for _, ended := range []string{"closed", "merged", "approved"} {
		if !PriorityEffect(priorityTree(ended), "td-epic").Independent {
			t.Errorf("with the parent %q its open children are claimed on their own", ended)
		}
	}
}

// TestParsePriorityScopeRefusesTheUnknown: an empty scope is the narrowest, so a caller that knows
// nothing of scopes keeps today's behaviour — but a misspelling is refused rather than quietly
// narrowed, which would report success for a cascade that never happened.
func TestParsePriorityScopeRefusesTheUnknown(t *testing.T) {
	for in, want := range map[string]PriorityScope{"": ScopeTask, "task": ScopeTask, "unrated": ScopeUnrated, "all": ScopeAll} {
		got, ok := ParsePriorityScope(in)
		if !ok || got != want {
			t.Errorf("ParsePriorityScope(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, bad := range []string{"subtasks", "tree", "children", "ALL"} {
		if _, ok := ParsePriorityScope(bad); ok {
			t.Errorf("ParsePriorityScope(%q) claimed to understand it", bad)
		}
	}
}
