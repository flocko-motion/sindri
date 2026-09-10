package task

import (
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"testing"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// TestClosingAForeignTaskLeavesItClosed is dain's loop, at the level it happens. SetStatus's closed
// branch called the source and returned, leaving the cached row OPEN — it trusted the source's next
// listing to show the close. That holds for an issue that really closes, and fails for a status
// living in the repo: the worker's tick is on its BRANCH while the listing reads the root. So
// AdvanceContainer, which reads this row, handed the same subtask back inside the same checkpoint.
func TestClosingAForeignTaskLeavesItClosed(t *testing.T) {
	a, ps, _ := ownedAct(t, "open")
	// A source that owns os- ids and whose ending is a no-op: the point here is what the HUB records
	// once the source has done its part, not what openspec does with a change.
	a.Sources = append(a.Sources, flowtest.ForeignSource{})
	if err := ps.UpsertTask(store.Task{ID: "os-1", Title: "a change (0/10)", Status: "open", Priority: "P0"}); err != nil {
		t.Fatal(err)
	}

	if err := a.pr().SetStatus(proj, "os-1", "closed"); err != nil {
		t.Fatalf("closing a foreign task: %v", err)
	}
	got, ok, _ := ps.GetTask("os-1")
	if !ok {
		t.Fatal("the task is gone; it should be closed, not removed")
	}
	if got.Status != "closed" {
		t.Errorf("status %q — the row the assigner reads still says open, so the subtask comes back", got.Status)
	}
	// And it must survive the rebuild, or the sync undoes it seconds later.
	if ov, err := ps.ClosedOverrides(); err != nil || !ov["os-1"] {
		t.Errorf("the ending was not kept across a sync: %v (err %v)", ov, err)
	}
}
