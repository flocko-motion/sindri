package fleet

import (
	"bytes"
	"github.com/flo-at/sindri/internal/hub/store"
	"strings"
	"testing"
)

// TestAnEditedTaskIsStillItsHolders: un-approving withdraws a task from the pools work is handed
// out FROM; it says nothing about finishing work already in hand. A worker stranded because a
// planner corrected a line in its brief would be the whole change made worthless.
func TestAnEditedTaskIsStillItsHolders(t *testing.T) {
	e, c, ps, id, deps := plannerOwnedTask(t, "approved")
	deps.Alive = true
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: id, Branch: id, Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code, err := e.taskAct().CmdEditTask(c, []string{id, "--body", "corrected"}, &out); code != 0 || err != nil {
		t.Fatalf("edit: code=%d err=%v out=%s", code, err, out.String())
	}
	d, err := e.AgentDirective(t.Context(), "proj", "eitri")
	if err != nil {
		t.Fatalf("directive: %v", err)
	}
	if !strings.Contains(d, id) || !strings.Contains(d, "submit") {
		t.Errorf("the holder should still be told to finish and submit %s, got: %s", id, d)
	}
}
