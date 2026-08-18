package agent

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/store"
)

// clearTestDeps is a minimal agent.Deps: enough for the arming paths, which never reach
// Rehydrate/RefreshTask/ProjectConfig.
type clearTestDeps struct{}

func (clearTestDeps) Notify()                                     {}
func (clearTestDeps) ContainerName(_, name string) string         { return "no-such-container-" + name }
func (clearTestDeps) ProjectRoot(string) string                   { return "" }
func (clearTestDeps) ProjectConfig(string) (config.Config, error) { return config.Config{}, nil }
func (clearTestDeps) ArchitectureDoc(string) string               { return "" }
func (clearTestDeps) RefreshTask(_, _ string) error               { return nil }
func (clearTestDeps) Rehydrate(_, _ string)                       {}

// armedFlag is the arming as the STORE holds it — what survives a hub restart, so it is read back
// rather than remembered from the call that set it.
func armedFlag(t *testing.T, ps *store.ProjectStore, name string) bool {
	t.Helper()
	a, ok, err := ps.GetAgent(name)
	if err != nil || !ok {
		t.Fatalf("reading %s back: ok=%v err=%v", name, ok, err)
	}
	return a.ClearArmed
}

// TestArmingWaitsForTheBoundary is the rule the feature rests on: an agent holding a leaf task has
// file-tree memory /clear would silently invalidate (claimLeaf resets the worktree on its OWN next
// claim, not this one), so the clear is ARMED and waits. It used to be refused outright, which left
// the user confirming into an error with nothing set.
func TestArmingWaitsForTheBoundary(t *testing.T) {
	_, st := newService(t)
	s := New(st, clearTestDeps{}, nil)
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: "td-abc123", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClearArmed("proj", "eitri", true); err != nil {
		t.Fatalf("arming a working agent must succeed — the waiting IS the feature: %v", err)
	}
	if !armedFlag(t, ps, "eitri") {
		t.Fatal("the arming must be recorded, or the user believes it is set when it is not")
	}
	if at, _ := s.AtLeafBoundary("proj", "eitri"); at {
		t.Error("an agent holding a leaf task is not at a boundary")
	}
	// And the gate the workflow reads agrees with the flag: this is what withholds new work.
	if !s.ClearArmed("proj", "eitri") {
		t.Error("ClearArmed must report what the store holds")
	}
}

// TestABoundaryIsNoLeafTaskAndNoReview: a feature in hand is not work a clear would cut into, since
// the clear fires between subtasks; a leaf subtask and an unverdicted review both are.
func TestABoundaryIsNoLeafTaskAndNoReview(t *testing.T) {
	_, st := newService(t)
	s := New(st, clearTestDeps{}, nil)
	ps := st.For("proj")
	for _, name := range []string{"eitri", "nori"} {
		if err := ps.PutAgent(store.Agent{Name: name, Role: "worker"}); err != nil {
			t.Fatal(err)
		}
	}
	// Between subtasks: the feature is still held, and that is a boundary.
	if err := ps.SetState(store.AgentState{Agent: "eitri", Container: "td-epic", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	if at, err := s.AtLeafBoundary("proj", "eitri"); err != nil || !at {
		t.Errorf("a feature between subtasks is a boundary: at=%v err=%v", at, err)
	}
	// Mid-subtask: not a boundary, feature or no feature.
	if err := ps.SetState(store.AgentState{Agent: "eitri", Container: "td-epic", Task: "td-leaf", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if at, _ := s.AtLeafBoundary("proj", "eitri"); at {
		t.Error("a subtask in hand is exactly what the clear must not cut into")
	}
	// A reviewer owing a verdict is mid-review.
	if err := ps.SetState(store.AgentState{Agent: "nori", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	if at, _ := s.AtLeafBoundary("proj", "nori"); !at {
		t.Fatal("a reviewer holding nothing is at a boundary")
	}
	if err := ps.PutPR(store.PR{ID: "pr-1", Agent: "eitri", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	id, err := ps.AddReview("pr-1", "check it")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.AssignReview(id, "nori"); err != nil {
		t.Fatal(err)
	}
	if at, _ := s.AtLeafBoundary("proj", "nori"); at {
		t.Error("a reviewer owing a verdict is not at a boundary")
	}
}

// TestDisarmingIsJustTheFlag: taking back an arming that never fired changes nothing else, and works
// on an agent whose pod is long gone — cancelling a destructive act is not itself destructive.
func TestDisarmingIsJustTheFlag(t *testing.T) {
	_, st := newService(t)
	s := New(st, clearTestDeps{}, nil)
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker", ClearArmed: true}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: "td-abc123", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClearArmed("proj", "eitri", false); err != nil {
		t.Fatalf("disarming must not fail: %v", err)
	}
	if armedFlag(t, ps, "eitri") {
		t.Error("the arming must be gone from the store, not just from the screen")
	}
}

// TestArmingAtABoundaryNeedsALivePod: an agent at a boundary fires at once, so a dead pod is a real
// error — there is nothing to send /clear into, and the boundary it would have waited for is the
// one it is standing at.
func TestArmingAtABoundaryNeedsALivePod(t *testing.T) {
	_, st := newService(t)
	s := New(st, clearTestDeps{}, nil)
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	err := s.SetClearArmed("proj", "eitri", true)
	if err == nil {
		t.Fatal("arming an agent with no running container should report that it cannot be cleared")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("error = %q, want it to name the missing pod", err.Error())
	}
	// And the flag is read back, not assumed: an error the user reads as "nothing happened" must
	// not leave the agent durably armed — armed, it would also be withheld from work by the gate.
	if armedFlag(t, ps, "eitri") {
		t.Error("a failed immediate clear must leave no arming behind it")
	}
}

// TestFireArmedClearsPassesOverAgentsStillWorking: the sweep is what lands an arming set minutes
// earlier, so it must be as careful as the arming was — a working agent is left alone, armed.
func TestFireArmedClearsPassesOverAgentsStillWorking(t *testing.T) {
	_, st := newService(t)
	s := New(st, clearTestDeps{}, nil)
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker", ClearArmed: true}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: "td-abc123", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	s.FireArmedClears("proj")
	if !armedFlag(t, ps, "eitri") {
		t.Error("a working agent's arming must survive the sweep — it fires at the boundary, not before")
	}
}
