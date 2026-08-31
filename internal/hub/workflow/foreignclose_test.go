package workflow

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// TestClosingAForeignTaskLeavesItClosed is dain's loop, at the level it happens. SetStatus's closed
// branch called the source and returned, leaving the cached row OPEN — it trusted the source's next
// listing to show the close. That holds for an issue that really closes, and fails for a status
// living in the repo: the worker's tick is on its BRANCH while the listing reads the root. So
// advanceContainer, which reads this row, handed the same subtask back inside the same checkpoint.
func TestClosingAForeignTaskLeavesItClosed(t *testing.T) {
	e, ps, _ := ownedEngine(t, "open")
	// A source that owns os- ids and whose ending is a no-op: the point here is what the HUB records
	// once the source has done its part, not what openspec does with a change.
	e.sources = append(e.sources, foreignSource{})
	if err := ps.UpsertTask(store.Task{ID: "os-1", Title: "a change (0/10)", Status: "open", Priority: "P0"}); err != nil {
		t.Fatal(err)
	}

	if err := e.SetStatus("proj", "os-1", "closed"); err != nil {
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

// foreignSource stands in for a task source whose status lives outside sindri's store.
type foreignSource struct{}

func (foreignSource) Name() string                            { return "foreign" }
func (foreignSource) Enabled(string) bool                     { return true }
func (foreignSource) ToolMissing(string) bool                 { return false }
func (foreignSource) Tasks(string, bool) ([]task.Task, error) { return nil, nil }
func (foreignSource) OnMerged(string, string, string) error   { return nil }
func (foreignSource) Finish(_, id string, _ bool) (bool, error) {
	return strings.HasPrefix(id, "os-"), nil
}
func (foreignSource) Comments(string, string) ([]task.Comment, bool, error) { return nil, false, nil }
func (foreignSource) AddComment(string, string, string) (bool, error)       { return false, nil }

// TestCheckpointDoesNotHandBackTheSubtaskItJustFinished is dain's loop end to end, on the path that
// actually runs it. CmdCheckpoint calls the source DIRECTLY rather than through SetStatus, so the
// local record — the one thing that closes a repo-held status for the hub — was skipped, and
// advanceContainer re-picked the subtask that had just been finished, inside the same call.
func TestCheckpointDoesNotHandBackTheSubtaskItJustFinished(t *testing.T) {
	e, ps, c, _ := gatedFeatureAlive(t)
	e.sources = append(e.sources, foreignSource{})
	// The held subtask is an os- id: a status that lives in the repo, which the hub cannot see move.
	if err := ps.UpsertTask(store.Task{ID: "os-1", Title: "a change (0/10)", Status: "open", Priority: "P0", ParentID: "td-EPIC"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: c.Agent, Task: "os-1", Container: "td-EPIC", Branch: "td-EPIC", Phase: "working"},
		store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if code, err := e.CmdCheckpoint(c, []string{"done"}, &out); code != 0 || err != nil {
		t.Fatalf("checkpoint: code=%d err=%v out=%s", code, err, out.String())
	}
	if got, _ := ps.GetState(c.Agent); got.Task == "os-1" {
		t.Errorf("the subtask just finished was handed straight back:\n%s", out.String())
	}
	if got, _, _ := ps.GetTask("os-1"); got.Status != "closed" {
		t.Errorf("os-1 is %q after being checkpointed — the assigner's row never closed", got.Status)
	}
}
