package workflow

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestARetiredAgentIsNotOfferedWork is oin's loop: AssignPendingWork pushed "sd-… is ready for you"
// at a retired agent every sweep, which answered "still retired, waiting quietly" each time. Its
// sibling nudgeIdleWorkers had asked agentBlocked since the commit that created this one, three lines
// away — the rule was applied in one and skipped in the other.
func TestARetiredAgentIsNotOfferedWork(t *testing.T) {
	st, ps := poolFixture(t)
	deps := &stubDeps{root: t.TempDir(), alive: true}
	e := New(st, deps)
	// Owned, not a bare cache row: AssignPendingWork syncs first, which rebuilds the cache.
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "sd-free", Title: "claimable", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	a := store.Agent{Name: "oin", Role: "worker", Workspace: "oin", Retired: true}
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "oin", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}

	e.AssignPendingWork("repo")
	for _, d := range deps.delivered {
		t.Errorf("a retired agent was offered work it is not eligible for: %+v", d)
	}

	// Back in service, the same sweep offers it — the guard must not silence a working agent.
	a.Retired = false
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	e.AssignPendingWork("repo")
	if len(deps.delivered) == 0 {
		t.Error("an unretired idle worker was left unaware of claimable work")
	}
}
