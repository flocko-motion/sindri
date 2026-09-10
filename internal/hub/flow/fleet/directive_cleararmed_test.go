// package: hub/flow/fleet / directive_cleararmed_test
// type:    logic (when an armed clear is allowed to land)
// job:     pin the boundary — a clear armed mid-subtask waits for the checkpoint, and fires at the
// leaf boundary the directive next answers from.
// limits:  the timing, which only a directive pass can show. What an armed clear WITHHOLDS is
// tested beside the claim (-> flow/task's task_act_cleararmed_test.go).
package fleet

import (
	"context"
	"github.com/flo-at/sindri/internal/hub/world/store"
	"testing"
)

// TestTheClearLandsBeforeTheNextSubtask: mid-subtask the clear waits; between subtasks, where a
// checkpoint leaves it, fireClearIfArmed fires it, same as the idle worker's own path.
func TestTheClearLandsBeforeTheNextSubtask(t *testing.T) {
	e, ps, deps := containerWorker(t, "working")
	ag, _, _ := ps.GetAgent("dvalin")
	ag.ClearArmed = true
	if err := ps.PutAgent(ag); err != nil {
		t.Fatal(err)
	}
	if _, err := e.AgentDirective(context.Background(), "repo", "dvalin"); err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if len(deps.Cleared) != 0 {
		t.Error("mid-subtask the agent keeps working; the clear must not fire before the checkpoint")
	}

	if err := ps.SetState(store.AgentState{Agent: "dvalin", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	fired, err := e.roleAct().FireClearIfArmed(t.Context(), "repo", "dvalin")
	if err != nil || !fired {
		t.Fatalf("fireClearIfArmed = (%v, %v), want it to fire now the agent is between subtasks", fired, err)
	}
	if len(deps.Cleared) != 1 || deps.Cleared[0] != "dvalin" {
		t.Errorf("cleared = %v, want the clear fired at the subtask boundary", deps.Cleared)
	}
}
