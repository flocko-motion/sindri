package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// plannerAt returns the directive a planner in the given phase is handed.
func plannerAt(t *testing.T, phase string) string {
	t.Helper()
	e, c, ps := plannerEngine(t, "td-1", "")
	if err := ps.PutAgent(store.Agent{Name: c.Agent, Role: "planner"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: c.Agent, Phase: phase}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	dir, err := e.AgentDirective(context.Background(), c.Project, c.Agent)
	if err != nil {
		t.Fatalf("AgentDirective(%s): %v", phase, err)
	}
	return dir
}

// TestPlannerIsNeverToldToWaitForAnAssignment is a planner's report, verbatim: "the hub still says
// nothing is assigned to me, and that a plan reaches me as a phased brief. Please assign it there."
// It had been handed work by the user in its own terminal and asked for it to be re-sent through the
// hub — because the directive named ONE route to it and forbade planning "on your own".
func TestPlannerIsNeverToldToWaitForAnAssignment(t *testing.T) {
	for _, phase := range []string{"", "idle", "planning"} {
		dir := plannerAt(t, phase)
		low := strings.ToLower(dir)
		for _, bad := range []string{"nothing is assigned", "while you wait", "until then"} {
			if strings.Contains(low, bad) {
				t.Errorf("phase %q: the directive says %q, which a planner reads as permission it must collect: %q", phase, bad, dir)
			}
		}
		// A planner acts on what the user says here. Nothing in the directive may deny that.
		if strings.Contains(low, "do not start planning") {
			t.Errorf("phase %q: forbidding planning outright refuses work the user handed over: %q", phase, dir)
		}
	}
}

// TestPlanningPlannerIsToldToCarryOn: AssignPlan sets phase "planning", and the directive ignored it
// — so the hub answered an agent it had just briefed with "nothing is assigned to you yet".
func TestPlanningPlannerIsToldToCarryOn(t *testing.T) {
	dir := plannerAt(t, "planning")
	if !strings.Contains(dir, "carry on") {
		t.Errorf("a planner mid-plan should be told to carry on, got %q", dir)
	}
	// GO still gates writing: carrying on is conversation, not filing.
	if !strings.Contains(dir, GoToken) {
		t.Errorf("the directive must keep the GO gate in view: %q", dir)
	}
}

// TestIdlePlannerStillGetsOriented: the fix must not cost the other half — a planner with genuinely
// nothing asked of it has reading to do, not an invitation to invent an assignment.
func TestIdlePlannerStillGetsOriented(t *testing.T) {
	dir := plannerAt(t, "idle")
	for _, want := range []string{"sindri task list", "openspec", GoToken} {
		if !strings.Contains(dir, want) {
			t.Errorf("an idle planner's directive should still name %q: %q", want, dir)
		}
	}
	if !strings.Contains(dir, "invent") {
		t.Errorf("it must still rule out inventing work: %q", dir)
	}
}
