package workflow

import (
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestCloseTaskStampsUpdatedAt: CloseTask updates the cached row directly rather than through
// RefreshTask (a full sync is too slow to run on every close), so it must stamp UpdatedAt itself —
// otherwise a task the "active" filter should catch the instant it closes would carry whatever
// stale timestamp the cache last held, or none at all.
func TestCloseTaskStampsUpdatedAt(t *testing.T) {
	e, ps, id := ownedEngine(t, "open")
	if err := ps.UpsertTask(store.Task{ID: id, Status: "open"}); err != nil {
		t.Fatal(err) // seed the read-model row CloseTask reads and rewrites
	}
	// RFC3339 truncates to whole seconds, so allow the second the close started in.
	before := time.Now().UTC().Add(-time.Second)

	if err := e.CloseTask("proj", id); err != nil {
		t.Fatal(err)
	}

	got, ok, err := ps.GetTask(id)
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if got.Status != "closed" {
		t.Fatalf("status = %q, want closed", got.Status)
	}
	at, err := time.Parse(time.RFC3339, got.UpdatedAt)
	if err != nil {
		t.Fatalf("UpdatedAt %q did not parse: %v", got.UpdatedAt, err)
	}
	if at.Before(before) {
		t.Errorf("UpdatedAt %s predates the close (started %s) — it should mark this moment", at, before)
	}
}
