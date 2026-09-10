package pr

import (
	"bytes"
	"github.com/flo-at/sindri/internal/hub/flow/run"
	"testing"

	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// testProject is the one repo these fixtures register.
const testProject = flowtest.TestProject

// proj is testProject under the name the fleet's tests have always used for it.
const proj = testProject

// newAct is a pull request's acting half over a real store and the fake hub every subject shares.
// The store is real on purpose: what a merge intent GUARANTEES is what survives a write.
func newAct(t *testing.T) (*Act, *store.ProjectStore) {
	a, ps, _ := newActWith(t, nil)
	return a, ps
}

// newActWith is newAct for a test that reads back what the hub was asked — a push, a wake, a
// comment posted on a task's thread.
func newActWith(t *testing.T, d *flowtest.Hub) (*Act, *store.ProjectStore, *flowtest.Hub) {
	t.Helper()
	if d == nil {
		d = &flowtest.Hub{}
	}
	c, ps := flowtest.Core(t, d)
	// The gate half of the run queue is this subject's own (-> core.Gates). flowtest cannot wire it
	// without importing pr, which would be a cycle back into the package under test.
	a := New(c)
	c.Gate = a
	return a, ps, d
}

// listAs runs `prs` as an agent and returns what it wrote. Direct rather than through the hub's exec
// surface: which verbs a state OFFERS is the registry's business and is tested there, so a listing
// test that went the long way would be asserting somebody else's rule.
func listAs(t *testing.T, a *Act, ps *store.ProjectStore, agent string, args ...string) (string, int) {
	t.Helper()
	ag, ok, err := ps.GetAgent(agent)
	if err != nil || !ok {
		t.Fatalf("no agent %q on the roster", agent)
	}
	var out bytes.Buffer
	code, err := a.CmdListPRs(registry.Caller{Project: testProject, Agent: agent, Role: ag.Role}, args, &out)
	if err != nil {
		t.Fatalf("prs %v: %v", args, err)
	}
	return out.String(), code
}

// newActOn is for a fixture that opened its own store and seeded it before wanting the acting half.
func newActOn(t *testing.T, st *store.Store, d *flowtest.Hub) *Act {
	t.Helper()
	if d.Root == "" {
		d.Root = t.TempDir()
	}
	c := flowtest.Over(st, d)
	a := New(c)
	c.Gate = a
	return a
}

// runAct is the one neighbour a merge intent's own tests reach into: a gate IS a queued run, so
// checking what a submit left behind means reading the queue. Setup and reading only — what the
// queue guarantees is tested in its own package.
//
// Nothing here reaches task or verbs on purpose: both import pr, so a test that needs one of them
// alongside this belongs in THAT package (-> flow/task's read_act_reviewer_test.go).
func (a *Act) runAct() *run.Act { return run.New(a.Core) }

// ownedAct seeds ONE task sindri owns, in both the owned table and the read model a sync leaves —
// the pair a PR settling under a task is judged against.
func ownedAct(t *testing.T, status string) (*Act, *store.ProjectStore, string) {
	t.Helper()
	a, ps := newAct(t)
	const id = "td-abc123"
	if err := ps.PutOwnedTask(store.OwnedTask{ID: id, Title: "a task", Status: status, Priority: "P2"}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := ps.UpsertTask(store.Task{ID: id, Title: "a task", Status: status, Priority: "P2"}); err != nil {
		t.Fatalf("seed cache row: %v", err)
	}
	return a, ps, id
}

// runQueuedGate takes the gate a submit or contribute just queued and runs it, so a test can assert
// what a PASSING gate unlocks rather than only that something was queued.
func runQueuedGate(t *testing.T, a *Act) {
	t.Helper()
	project, id, ok := a.runAct().NextQueuedRun()
	if !ok {
		t.Fatal("expected a queued gate run")
	}
	if err := a.runAct().ExecuteRun(t.Context(), project, id); err != nil {
		t.Fatalf("ExecuteRun(%s): %v", id, err)
	}
}
