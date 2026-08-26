package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// featureWorker seeds a worker on feature td-EPIC with one subtask, in a real worktree so submit can
// commit. openChild says whether that subtask is still open. Returns deps too, so a test can read
// what was injected once a queued gate lands (-> runQueuedGate).
func featureWorker(t *testing.T, openChild bool) (*Engine, *store.ProjectStore, registry.Caller, *stubDeps) {
	t.Helper()
	const agent = "dain"
	root, _ := newWorkRepo(t, agent, "td-EPIC")
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: filepath.Join(".worktrees", agent)}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	child := store.Task{ID: "td-1", Title: "a subtask", Status: "closed", Priority: "P1", ParentID: "td-EPIC"}
	if openChild {
		child.Status = "open"
	}
	for _, task := range []store.Task{
		{ID: "td-EPIC", Title: "a feature", Status: "open", Priority: "P1", Type: "epic"}, child,
	} {
		if err := ps.UpsertTask(task); err != nil {
			t.Fatalf("seed %s: %v", task.ID, err)
		}
	}
	phase := "idle" // every subtask checkpointed: the feature is built and waiting to go up
	if openChild {
		phase = "working"
	}
	if err := ps.SetState(store.AgentState{
		Agent: agent, Container: "td-EPIC", Branch: "td-EPIC", Task: "td-1", Phase: phase,
	}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	// Work on the branch for submit to record.
	if err := os.WriteFile(filepath.Join(root, ".worktrees", agent, "feature.txt"), []byte("built\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{root: root}
	return New(st, deps), ps, registry.Caller{Project: "repo", Agent: agent, Role: "worker", Phase: phase}, deps
}

// TestFinishedFeatureSubmitsItself is the answer to "why can't dain submit his work?". A parent task
// with children is just a hierarchy: it changes the unit under review — one PR for the branch every
// subtask was checkpointed onto — and nothing about who puts it up. The worker that built it submits
// it, exactly as it would a task of its own; a human opening the PR by hand is not a step.
func TestFinishedFeatureSubmitsItself(t *testing.T) {
	e, ps, c, _ := featureWorker(t, false)
	if code, out := submitAll(t, e, c, "separate the front-ends from the hub"); code != 0 {
		t.Fatalf("submit: code=%d out=%s", code, out)
	}
	runQueuedGate(t, e)
	pr, ok, _ := ps.GetPR("pr-td-EPIC")
	if !ok {
		t.Fatal("submitting a finished feature should open a PR for the feature")
	}
	if pr.Task != "td-EPIC" || pr.Branch != "td-EPIC" {
		t.Errorf("the PR should cover the feature branch, got task=%q branch=%q", pr.Task, pr.Branch)
	}
	// It stays the feature's worker while the PR is out, so a verdict comes back to the right loop.
	if s, _ := ps.GetState("dain"); s.Phase != "submitted" || s.Container != "td-EPIC" {
		t.Errorf("state after submit = {phase:%q container:%q}, want {submitted td-EPIC}", s.Phase, s.Container)
	}
}

// TestUnfinishedFeatureIsRefusedWithWhatIsLeft: one PR per feature means submitting early would put
// an incomplete branch under review, so the refusal names the subtask still open and the verb that
// clears it.
func TestUnfinishedFeatureIsRefusedWithWhatIsLeft(t *testing.T) {
	e, ps, c, _ := featureWorker(t, true)
	var out strings.Builder
	code, _ := e.CmdSubmit(c, []string{"early"}, &out)
	if code == 0 {
		t.Error("a feature with open subtasks must not go up")
	}
	if _, ok, _ := ps.GetPR("pr-td-EPIC"); ok {
		t.Error("no PR should exist for an unfinished feature")
	}
	for _, want := range []string{"td-EPIC", "td-1", "`sindri checkpoint"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the refusal should carry %q: %s", want, out.String())
		}
	}
}
