package workflow

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestAnAgentAwaitingAVerdictIsHandedNothing: austri's pr-sd-a47b61 was rejected — its work was
// unfinished and its state row empty — so the gate read it as free and gave it a second task.
func TestAnAgentAwaitingAVerdictIsHandedNothing(t *testing.T) {
	e, ps, _ := ownedEngine(t, "open")
	if err := ps.PutAgent(store.Agent{Name: "durin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "durin", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-sd-9", Task: "sd-9", Agent: "durin", Status: "rejected"}); err != nil {
		t.Fatal(err)
	}

	why := e.agentBlocked(ps, "proj", "durin")
	if why == "" {
		t.Fatal("an author waiting on a rejected PR was offered work; it still owes that task")
	}
	if !strings.Contains(why, "pr-sd-9") {
		t.Errorf("the reason %q never names the PR that holds it", why)
	}
}

// TestCheckpointRefusedWhileThePRIsUnlanded: a rejection returns the work in phase "working", which
// is the shape a checkpoint reads as finished — so austri checkpointed straight past one, closing
// the task and freeing itself. A task ends when its PR lands.
func TestCheckpointRefusedWhileThePRIsUnlanded(t *testing.T) {
	e, ps, _, caller := submitEngine(t)
	if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "sd-1", Branch: "sd-1", Container: "sd-0", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-sd-1", Task: "sd-1", Agent: "bombur", Status: "rejected"}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	code, err := e.CmdCheckpoint(caller, []string{"done"}, &out)
	if err != nil {
		t.Fatalf("CmdCheckpoint: %v", err)
	}
	if code == 0 {
		t.Fatalf("the checkpoint closed a task whose PR had not landed; said %q", out.String())
	}
	if got, _, _ := ps.GetTask("sd-1"); got.Status == "closed" {
		t.Error("the task was closed anyway")
	}
}
