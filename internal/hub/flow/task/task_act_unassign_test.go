package task

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// TestUnassignTaskTakesALiveHolderOffIt: asking for a task back gets it back. The verb used to
// refuse while the holder's pod was up — "stop or delete it first" — which is the human being handed
// a chore in place of an answer, and leaves the board claiming an agent is on work already taken
// away until they do it. Liveness was never even the question it looked like: Up means the pod is
// running, so an agent sitting at an idle prompt was refused on the grounds of "working".
func TestUnassignTaskTakesALiveHolderOffIt(t *testing.T) {
	d := &flowtest.Hub{} // live: the default, and the case that used to be refused
	a, ps := newAct(t, d)
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-solo", Title: "a task", Status: "in_progress", Priority: "P2"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	flowtest.Place(t, ps, store.AgentState{Agent: "dvalin", Task: "td-solo", Branch: "td-solo", Phase: "working"})
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "td-solo", Agent: "dvalin", Branch: "td-solo", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}

	if err := a.UnassignTask(proj, "td-solo"); err != nil {
		t.Fatalf("UnassignTask over a live holder: %v", err)
	}

	got, err := ps.GetState("dvalin")
	if err != nil {
		t.Fatal(err)
	}
	if got.Task != "" {
		t.Errorf("the task must be off the holder, got Task=%q", got.Task)
	}
	// ESC, then the notice: told what happened and stopped doing it, or it works on against a task
	// the backlog has already handed to somebody else.
	if len(d.Interrupted) == 0 {
		t.Error("a live holder must be interrupted, so the notice lands on an idle prompt")
	}
	if !strings.Contains(strings.Join(d.Said(), "\n"), "td-solo") {
		t.Errorf("the holder must be told which task was taken back, said: %q", d.Said())
	}
	// The PR goes with the work. Left open it binds the agent to what it no longer holds — AwaitingPR
	// reads an unsettled PR as held work, so the directive keeps sending it back to the same task.
	pr, ok, err := ps.GetPR("pr-1")
	if err != nil || !ok {
		t.Fatalf("read pr-1: ok=%v err=%v", ok, err)
	}
	if pr.Status != "scrapped" {
		t.Errorf("the released agent's PR must be scrapped, got %q", pr.Status)
	}
}
