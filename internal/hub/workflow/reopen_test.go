package workflow

import "testing"

// TestReopenTaskRestoresAClosedOwnedTask: the mechanics ReopenTask exists for — a closed task
// sindri owns goes back to open, and its priority is untouched (which alone can make it
// immediately claimable again).
func TestReopenTaskRestoresAClosedOwnedTask(t *testing.T) {
	e, ps, id := ownedEngine(t, "closed")

	if err := e.ReopenTask("proj", id); err != nil {
		t.Fatalf("ReopenTask: %v", err)
	}
	got, ok, err := ps.OwnedTask(id)
	if err != nil || !ok {
		t.Fatalf("OwnedTask: ok=%v err=%v", ok, err)
	}
	if got.Status != "open" {
		t.Errorf("status = %q, want open", got.Status)
	}
	if got.Priority != "P2" {
		t.Errorf("priority = %q, want it left exactly as it stood (P2) — that alone can release the task", got.Priority)
	}
	// The read-model row (what task list/TUI actually show) must agree, or the task reads closed
	// everywhere except the one table nobody displays.
	cached, ok, err := ps.GetTask(id)
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if cached.Status != "open" {
		t.Errorf("cached status = %q, want open", cached.Status)
	}
}

// TestReopenTaskRefusesATaskThatIsNotClosed: nothing to reopen on a task that never closed —
// silently no-op'ing here would make a mistyped id look like it worked.
func TestReopenTaskRefusesATaskThatIsNotClosed(t *testing.T) {
	e, _, id := ownedEngine(t, "open")

	err := e.ReopenTask("proj", id)
	if err == nil {
		t.Fatal("expected an error reopening a task that isn't closed")
	}
}

// TestReopenTaskRefusesAnUnownedID: an openspec change or a GitHub issue keeps its status at its
// own source — writing "open" into sindri's store here would only desync from what that source
// actually reports, so REOPEN scope is sindri-owned ids only (sd-/td-).
func TestReopenTaskRefusesAnUnownedID(t *testing.T) {
	e, _, _ := ownedEngine(t, "closed")

	for _, id := range []string{"os-abc123", "gh-42"} {
		if err := e.ReopenTask("proj", id); err == nil {
			t.Errorf("%s: expected a refusal — its status comes from its own source, not sindri's store", id)
		}
	}
}
