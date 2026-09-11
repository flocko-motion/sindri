package fleet

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// TestContributeThenMergeKeepsTaskOpen is the interim-contribution happy path:
// `contribute` records a GATED interim PR (open, no reviewer requested), and merging
// it keeps the task open and puts the worker straight back to "working" on the SAME
// task — the point of a mid-task contribution.
func TestContributeThenMergeKeepsTaskOpen(t *testing.T) {
	const agent, task, branch = "wrk", "td-1", "td-1"
	root, base := flowtest.WorkRepo(t, agent, branch)

	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: filepath.Join(".worktrees", agent)}); err != nil {
		t.Fatal(err)
	}
	flowtest.Place(t, ps, store.AgentState{Agent: agent, Task: task, Branch: branch, Phase: "working"})
	// The worker made progress it wants to land mid-task.
	if err := os.WriteFile(filepath.Join(root, ".worktrees", agent, "work.txt"), []byte("progress\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := newEngine(t, st, &flowtest.Hub{Root: root})
	caller := registry.Caller{Project: "repo", Agent: agent, Role: "worker", HasTask: true, Phase: "working"}
	if code, err := e.prAct().CmdContribute(caller, []string{"checkpoint"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdContribute: code=%d err=%v", code, err)
	}
	runQueuedGate(t, e)

	pr, ok, _ := ps.GetPR("pr-" + task)
	if !ok {
		t.Fatal("contribute should have created pr-td-1")
	}
	if pr.Kind != "interim" {
		t.Fatalf("PR kind = %q, want interim", pr.Kind)
	}
	if pr.Status != "open" {
		t.Fatalf("PR status = %q, want open (gated, awaiting the user)", pr.Status)
	}
	if revs, _ := ps.Reviews(pr.ID); len(revs) != 0 {
		t.Fatalf("interim PR must not request a reviewer, got %d review(s)", len(revs))
	}
	// The worker waits after contributing.
	if s, _ := ps.GetState(agent); s.Phase != worker.Submitted {
		t.Fatalf("worker phase after contribute = %q, want submitted", s.Phase)
	}

	// The user approves, then merges.
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	if _, err := e.prAct().Merge("repo", pr.ID); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	merged, _, _ := ps.GetPR(pr.ID)
	if merged.Status != "merged" {
		t.Fatalf("PR status after merge = %q, want merged", merged.Status)
	}
	// The task stays with the worker and it's back to working — NOT idled/closed. Where it stands is
	// its own map's to write, off the milestone this merge just landed.
	e.Look("repo", agent)
	s, _ := ps.GetState(agent)
	if s.Phase != worker.Working || s.Task != task {
		t.Fatalf("worker after interim merge = {phase:%q task:%q}, want {working %s}", s.Phase, s.Task, task)
	}
	_ = base
}

// TestFeatureContributionMergeKeepsTheFeature: the merge lands the work and the worker carries on
// with the same feature — the whole point of an interim landing. Closing td-EPIC here would finish a
// feature that still has subtasks to do.
func TestFeatureContributionMergeKeepsTheFeature(t *testing.T) {
	e, ps, c, _ := featureWorker(t, true)
	if code, err := e.prAct().CmdContribute(c, nil, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdContribute: code=%d err=%v", code, err)
	}
	runQueuedGate(t, e)
	pr, _, _ := ps.GetPR("pr-td-EPIC")
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	if _, err := e.prAct().Merge("repo", pr.ID); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	e.Look("repo", "dain")
	st, _ := ps.GetState("dain")
	if st.Container != "td-EPIC" || st.Branch != "td-EPIC" || st.Phase != worker.Working {
		t.Errorf("state = {container:%q branch:%q phase:%q}, want it back at work on the feature",
			st.Container, st.Branch, st.Phase)
	}
	feature, _, _ := ps.GetTask("td-EPIC")
	if feature.Status != "open" {
		t.Errorf("feature status = %q — an instalment of a feature must not finish it", feature.Status)
	}
	// And it is still claimable work: a merged interim PR is not the feature having landed.
	flowtest.Place(t, ps, store.AgentState{Agent: "dain", Phase: "idle"})
	open, err := ps.OpenContainers()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].ID != "td-EPIC" {
		t.Errorf("open containers = %v, want td-EPIC still on offer", open)
	}
}
