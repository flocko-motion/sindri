package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// containerWorker seeds a worker holding feature td-EPIC and working subtask td-1, the state a
// checkpoint leaves behind, and returns the engine, the project store and the deps stub.
func containerWorker(t *testing.T, phase string) (*Engine, *store.ProjectStore, *stubDeps) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-EPIC", Title: "a feature", Status: "open", Type: "epic"}); err != nil {
		t.Fatalf("seed container: %v", err)
	}
	if err := ps.SetState(store.AgentState{
		Agent: "dvalin", Container: "td-EPIC", Branch: "td-EPIC", Task: "td-1", Phase: phase,
	}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	deps := &stubDeps{root: t.TempDir()}
	return New(st, deps), ps, deps
}

// TestContainerDirectiveNamesCheckpoint is the bug a worker reported from inside a feature: only the
// claim named `checkpoint`, and every `sindri` after it fell through to the standalone-task directive
// asking for a `submit` the container surface hides. The agent had to work out for itself which verb
// it actually held.
func TestContainerDirectiveNamesCheckpoint(t *testing.T) {
	e, _, _ := containerWorker(t, "working")
	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "`sindri checkpoint") {
		t.Errorf("a feature worker's directive should name checkpoint: %q", dir)
	}
	// It may say where the branch is headed — an agent that doesn't know a feature ends in a PR
	// stops when the code is written — but the submit must be pinned to its condition, or it reads
	// as something to do now and the surface refuses it.
	if strings.Contains(dir, "`sindri submit") && !strings.Contains(dir, "never per subtask") {
		t.Errorf("mid-feature, a submit must be qualified rather than offered: %q", dir)
	}
	if !strings.Contains(dir, "td-1") || !strings.Contains(dir, "td-EPIC") {
		t.Errorf("the directive should name both the subtask and the feature: %q", dir)
	}
}

// TestRejectedFeatureKeepsTheFeature: SetState writes the whole row, so the rejection used to drop
// the container — the worker fell out of the feature loop, went idle and claimed unrelated work,
// leaving the branch its checkpointed subtasks were on. It stays on the feature, and is sent round
// the same loop a rejected leaf task follows: fix the branch you're on, submit it again.
func TestRejectedFeatureKeepsTheFeature(t *testing.T) {
	e, ps, deps := containerWorker(t, "submitted")
	if err := ps.PutPR(store.PR{
		ID: "pr-td-EPIC", Task: "td-EPIC", Agent: "dvalin", Branch: "td-EPIC", Status: "open",
	}); err != nil {
		t.Fatalf("put PR: %v", err)
	}
	if err := e.RejectPR("repo", "pr-td-EPIC", "not yet"); err != nil {
		t.Fatalf("RejectPR: %v", err)
	}
	st, _ := ps.GetState("dvalin")
	if st.Container != "td-EPIC" {
		t.Errorf("container after rejection = %q, want td-EPIC — the feature is not abandoned", st.Container)
	}
	if len(deps.injectedText) != 1 {
		t.Fatalf("want one message to the author, got %d", len(deps.injectedText))
	}
	msg := deps.injectedText[0]
	for _, want := range []string{"td-EPIC", "not yet", "`sindri submit"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the rejection should carry %q: %q", want, msg)
		}
	}
	// And the directive it gets next keeps it on the feature, pointed at the same resubmit.
	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "`sindri submit") {
		t.Errorf("the post-rejection directive should point at the resubmit: %q", dir)
	}
}
