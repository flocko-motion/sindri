package fleet

import (
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// TestAStalledWorkerThatEscalatesIsEscalated is what declaring the escalation once, on the region,
// bought: stalled never listed it, so a worker that stopped on a question while its screen stood
// still stayed "stalled" — prodded — until the screen moved again. It inherits the exit now, first.
func TestAStalledWorkerThatEscalatesIsEscalated(t *testing.T) {
	// The screen stays still throughout, so nothing else moves it on first: before the region held
	// the escalation, this worker stayed "stalled" however long the question stood.
	d := &flowtest.Hub{Root: t.TempDir(), StillFor: time.Hour}
	e, ps, _ := podFleet(t, d, store.Agent{Name: "dain", Role: "worker", Workspace: ".worktrees/dain"})
	flowtest.Place(t, ps, store.AgentState{Agent: "dain", Task: "td-1", Branch: "td-1", Phase: worker.Stalled})
	if err := ps.SetEscalation("dain", "which of the two schemas is meant?"); err != nil {
		t.Fatal(err)
	}

	e.Look("repo", "dain")

	if st, _ := ps.GetState("dain"); st.Phase != worker.Escalated {
		t.Errorf("a stalled worker with a question standing is in %q, want %q", st.Phase, worker.Escalated)
	}
}
