package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestRetierDueWithholdsAMismatchedTask: the highest-priority claimable task needs a different
// model than the one running — the gate must withhold it for the off-tick sweep, not hand it over.
func TestRetierDueWithholdsAMismatchedTask(t *testing.T) {
	deps := &stubDeps{
		currentModel: "claude-haiku-4-5",
		tierModels:   map[string]string{"junior": "claude-haiku-4-5", "senior": "claude-opus-5"},
	}
	e, ps := idleWorkerWithOpenTask(t, deps)
	if err := ps.SetOwnedTier("td-abc123", "senior"); err != nil {
		t.Fatal(err)
	}
	e.refreshCachedTask("repo", "td-abc123")

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "senior") {
		t.Errorf("directive = %q, want it to say the task needs the senior tier", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "" {
		t.Errorf("a mismatched task was handed over anyway: %q", st.Task)
	}
}

// TestRetierDueLetsAMatchingTaskThrough is the control: nothing about the tier check should stop
// an ordinary claim when the model already matches.
func TestRetierDueLetsAMatchingTaskThrough(t *testing.T) {
	deps := &stubDeps{
		currentModel: "claude-opus-5",
		tierModels:   map[string]string{"senior": "claude-opus-5"},
	}
	e, ps := idleWorkerWithOpenTask(t, deps)
	if err := ps.SetOwnedTier("td-abc123", "senior"); err != nil {
		t.Fatal(err)
	}
	e.refreshCachedTask("repo", "td-abc123")

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "td-abc123") {
		t.Errorf("directive = %q, want the task claimed — the model already matches", dir)
	}
}

// TestFireDueRetiersChangesAMismatchedIdleWorker runs the sweep end to end against stubDeps: a
// free, idle worker whose next claimable task needs a different model gets SetModel called for it.
func TestFireDueRetiersChangesAMismatchedIdleWorker(t *testing.T) {
	deps := &stubDeps{
		alive:        true,
		currentModel: "claude-haiku-4-5",
		tierModels:   map[string]string{"senior": "claude-opus-5"},
	}
	e, ps := idleWorkerWithOpenTask(t, deps)
	if err := ps.SetOwnedTier("td-abc123", "senior"); err != nil {
		t.Fatal(err)
	}
	e.refreshCachedTask("repo", "td-abc123")

	e.FireDueRetiers("repo")

	if len(deps.modelSet) != 1 || deps.modelSet[0] != "dvalin=claude-opus-5" {
		t.Errorf("modelSet = %v, want exactly one SetModel(dvalin, claude-opus-5)", deps.modelSet)
	}
}

// TestFireDueRetiersLeavesARetiredWorkerAlone: retirement exempts every automatic behaviour.
func TestFireDueRetiersLeavesARetiredWorkerAlone(t *testing.T) {
	deps := &stubDeps{
		alive:        true,
		currentModel: "claude-haiku-4-5",
		tierModels:   map[string]string{"senior": "claude-opus-5"},
	}
	e, ps := idleWorkerWithOpenTask(t, deps)
	a, _, err := ps.GetAgent("dvalin")
	if err != nil {
		t.Fatal(err)
	}
	a.Retired = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetOwnedTier("td-abc123", "senior"); err != nil {
		t.Fatal(err)
	}
	e.refreshCachedTask("repo", "td-abc123")

	e.FireDueRetiers("repo")

	if len(deps.modelSet) != 0 {
		t.Errorf("a retired worker was retiered anyway: %v", deps.modelSet)
	}
}

// TestFireDueRetiersLeavesABusyWorkerAlone: mid-task is never a safe moment for a model change,
// whatever else is true.
func TestFireDueRetiersLeavesABusyWorkerAlone(t *testing.T) {
	deps := &stubDeps{
		alive:        true,
		currentModel: "claude-haiku-4-5",
		tierModels:   map[string]string{"senior": "claude-opus-5"},
	}
	e, ps := idleWorkerWithOpenTask(t, deps)
	if err := ps.SetOwnedTier("td-abc123", "senior"); err != nil {
		t.Fatal(err)
	}
	e.refreshCachedTask("repo", "td-abc123")
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-abc123", Phase: "working"}); err != nil {
		t.Fatal(err)
	}

	e.FireDueRetiers("repo")

	if len(deps.modelSet) != 0 {
		t.Errorf("a busy worker was retiered anyway: %v", deps.modelSet)
	}
}
