package task

import (
	agentflow "github.com/flo-at/sindri/internal/hub/flow/agent"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// proj is the one repo these fixtures register, named as the fleet's tests have always named it.
const proj = flowtest.TestProject

// newAct is a task's acting half over a real store and the fake hub every subject shares.
func newAct(t *testing.T, d *flowtest.Hub) (*Act, *store.ProjectStore) {
	t.Helper()
	if d == nil {
		d = &flowtest.Hub{}
	}
	c, ps := flowtest.Core(t, d)
	return New(c), ps
}

// ownedAct seeds ONE task sindri owns, in both the owned table and the read model a sync leaves —
// the pair the repair sweep exists to keep in step.
func ownedAct(t *testing.T, status string) (*Act, *store.ProjectStore, string) {
	t.Helper()
	a, ps := newAct(t, nil)
	const id = "td-abc123"
	if err := ps.PutOwnedTask(store.OwnedTask{ID: id, Title: "a task", Status: status, Priority: "P2"}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := ps.UpsertTask(store.Task{ID: id, Title: "a task", Status: status, Priority: "P2"}); err != nil {
		t.Fatalf("seed cache row: %v", err)
	}
	return a, ps, id
}

// plannerAct seeds one flat proposal and the planner that authored it.
func plannerAct(t *testing.T, id, approval string) (*Act, registry.Caller, *store.ProjectStore) {
	t.Helper()
	a, ps := newAct(t, nil)
	if err := ps.UpsertTask(store.Task{ID: id, Title: "flat proposal", Status: "open"}); err != nil {
		t.Fatalf("upsert task: %v", err)
	}
	if approval != "" {
		if err := ps.SetApproval(id, approval, ""); err != nil {
			t.Fatalf("set approval: %v", err)
		}
	}
	return a, registry.Caller{Project: proj, Agent: "galar", Role: "planner"}, ps
}

// workerAct seeds a backlog and a worker standing where the caller says, for the reads that answer
// differently depending on what it already holds.
func workerAct(t *testing.T, tasks []store.Task, container, current string) (*Act, registry.Caller) {
	return workerActComments(t, tasks, container, current, nil)
}

// workerActComments is workerAct with a seeded comment thread per task id — the thread lives outside
// the task row, so the fake hub serves it rather than the store.
func workerActComments(t *testing.T, tasks []store.Task, container, current string,
	comments map[string][]store.Comment) (*Act, registry.Caller) {
	t.Helper()
	a, ps := newAct(t, &flowtest.Hub{Comments: comments})
	for _, task := range tasks {
		if err := ps.UpsertTask(task); err != nil {
			t.Fatalf("upsert %s: %v", task.ID, err)
		}
	}
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if container != "" || current != "" {
		st := store.AgentState{Agent: "dvalin", Container: container, Task: current, Phase: "working"}
		if err := ps.SetState(st, store.ReasonClaimed, "test setup"); err != nil {
			t.Fatalf("set state: %v", err)
		}
	}
	return a, registry.Caller{Project: proj, Agent: "dvalin", Role: "worker"}
}

// newActWith is for a fixture that opened its own store before this one existed — it seeds rows,
// then wants the acting half over them. Everything else takes newAct.
func newActWith(t *testing.T, st *store.Store, d *flowtest.Hub) *Act {
	t.Helper()
	if d.Root == "" {
		d.Root = t.TempDir()
	}
	return New(flowtest.Over(st, d))
}

// newActWith2 is newActWith for a fixture that opened its own store and seeded it first.
func newActWith2(t *testing.T, st *store.Store, d *flowtest.Hub) *Act {
	t.Helper()
	if d.Root == "" {
		d.Root = t.TempDir()
	}
	return New(flowtest.Over(st, d))
}

// agent is the acting half of an agent's own flow: preparing the session a claim lands in is the
// agent's business, and a test about WITHHOLDING a claim has to reach both.
func (a *Act) agent() *agentflow.Act { return agentflow.New(a.Core) }

// poolFixture seeds one open PR with an unclaimed review row — the shape a pooled reviewer is
// handed, and the scope a reviewer's task reads are judged against.
func poolFixture(t *testing.T) (*store.Store, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For(proj)
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "td-1", Agent: "bombur", Branch: "sd-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddReview("pr-1", "review it"); err != nil {
		t.Fatal(err)
	}
	return st, ps
}
