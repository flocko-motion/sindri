// package: hub/flow/fleet / checkpoint_subtask_test
// type:    logic (what a checkpoint hands over next)
// job:     pin that finishing a subtask never hands the same one back — the shape that had a worker
// re-doing work it had just closed.
// limits:  the hand-over, which takes a machine pass to see.
package fleet

import (
	"bytes"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/world/store"
	"testing"
)

// TestCheckpointDoesNotHandBackTheSubtaskItJustFinished is dain's loop end to end, on the path that
// actually runs it. CmdCheckpoint calls the source DIRECTLY rather than through SetStatus, so the
// local record — the one thing that closes a repo-held status for the hub — was skipped, and
// AdvanceContainer re-picked the subtask that had just been finished, inside the same call.
func TestCheckpointDoesNotHandBackTheSubtaskItJustFinished(t *testing.T) {
	e, ps, c, _ := gatedFeatureAlive(t)
	e.Sources = append(e.Sources, flowtest.ForeignSource{})
	// The held subtask is an os- id: a status that lives in the repo, which the hub cannot see move.
	if err := ps.UpsertTask(store.Task{ID: "os-1", Title: "a change (0/10)", Status: "open", Priority: "P0", ParentID: "td-EPIC"}); err != nil {
		t.Fatal(err)
	}
	flowtest.Place(t, ps, store.AgentState{Agent: c.Agent, Task: "os-1", Container: "td-EPIC", Branch: "td-EPIC", Phase: "working"})

	var out bytes.Buffer
	if code, err := e.taskAct().CmdCheckpoint(c, []string{"done"}, &out); code != 0 || err != nil {
		t.Fatalf("checkpoint: code=%d err=%v out=%s", code, err, out.String())
	}
	if got, _ := ps.GetState(c.Agent); got.Task == "os-1" {
		t.Errorf("the subtask just finished was handed straight back:\n%s", out.String())
	}
	if got, _, _ := ps.GetTask("os-1"); got.Status != "closed" {
		t.Errorf("os-1 is %q after being checkpointed — the assigner's row never closed", got.Status)
	}
}
