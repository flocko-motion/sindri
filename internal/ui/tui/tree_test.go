package tui

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// rowsAt builds arranged rows from a depth sequence — the only field the derivation reads.
func rowsAt(depths ...int) []api.TaskRow {
	out := make([]api.TaskRow, len(depths))
	for i, d := range depths {
		out[i] = api.TaskRow{Depth: d}
	}
	return out
}

// TestLastSiblingsMatchesTheArrangement pins the derivation that replaced a wire field: a row is
// last when no later row sits at its depth before a shallower one, which is what "no later sibling"
// means once the tree has been flattened.
func TestLastSiblingsMatchesTheArrangement(t *testing.T) {
	for _, c := range []struct {
		what   string
		depths []int
		want   []bool
	}{
		{"nothing to draw", nil, nil},
		{"one root", []int{0}, []bool{true}},
		{"flat siblings — only the final one closes", []int{0, 0, 0}, []bool{false, false, true}},
		{
			"a parent, its two children, then an uncle: the deeper rows close their own level",
			[]int{0, 1, 1, 0},
			[]bool{false, false, true, true},
		},
		{
			"a subtree between siblings must not make the first parent look last",
			[]int{0, 1, 2, 2, 1, 0},
			[]bool{false, false, false, true, true, true},
		},
		{
			"returning to a shallower depth reopens it for later siblings",
			[]int{0, 1, 0, 1},
			[]bool{false, true, true, true},
		},
	} {
		got := lastSiblings(rowsAt(c.depths...))
		if len(got) != len(c.want) {
			t.Errorf("%s: got %d results, want %d", c.what, len(got), len(c.want))
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: row %d (depth %d) = %v, want %v\n  all: %v", c.what, i, c.depths[i], got[i], c.want[i], got)
				break
			}
		}
	}
}

// TestLastSiblingsAgreesWithArrangeTasks is the real check: the derivation must reproduce, for a
// tree the hub actually arranges, what the dropped wire field used to carry — the last child of
// every parent, and nothing else.
func TestLastSiblingsAgreesWithArrangeTasks(t *testing.T) {
	tasks := []api.Task{
		{ID: "td-a", Priority: "P1"},
		{ID: "td-a1", ParentID: "td-a", Priority: "P1"},
		{ID: "td-a2", ParentID: "td-a", Priority: "P2"},
		{ID: "td-a2x", ParentID: "td-a2", Priority: "P1"},
		{ID: "td-b", Priority: "P2"},
	}
	rows := api.ArrangeTasks(tasks, nil)
	if len(rows) != len(tasks) {
		t.Fatalf("expected every task arranged, got %d of %d", len(rows), len(tasks))
	}

	// Recompute the old rule directly from the tree: a row is last when it is its parent's final
	// child in the arrangement.
	lastChild := map[string]string{} // parent id -> its final child in row order
	parentOf := map[string]string{}
	for _, tk := range tasks {
		parentOf[tk.ID] = tk.ParentID
	}
	for _, r := range rows {
		lastChild[parentOf[r.ID]] = r.ID
	}

	got := lastSiblings(rows)
	for i, r := range rows {
		want := lastChild[parentOf[r.ID]] == r.ID
		if got[i] != want {
			t.Errorf("%s (depth %d): derived %v, but it %s its parent's last child",
				r.ID, r.Depth, got[i], map[bool]string{true: "is", false: "is not"}[want])
		}
	}
}
