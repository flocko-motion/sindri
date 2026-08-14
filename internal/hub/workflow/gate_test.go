package workflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

func TestQueuePositionsRanksGateRunsFirst(t *testing.T) {
	runs := []api.Run{
		{ID: "explore-p0", Status: "queued", Priority: "P0", CreatedAt: "2026-01-01T00:00:00Z"},
		{ID: "gate-later", Status: "queued", Kind: "submit", CreatedAt: "2026-01-01T00:00:05Z"},
		{ID: "gate-earlier", Status: "queued", Kind: "contribute", CreatedAt: "2026-01-01T00:00:01Z"},
		{ID: "explore-p1", Status: "queued", Priority: "P1", CreatedAt: "2026-01-01T00:00:02Z"},
	}
	pos := queuePositions(runs)
	want := map[string]int{"gate-earlier": 1, "gate-later": 2, "explore-p0": 3, "explore-p1": 4}
	for id, w := range want {
		if pos[id] != w {
			t.Errorf("position[%q] = %d, want %d (got %v)", id, pos[id], w, pos)
		}
	}
}

// TestEnqueueGateSnapshotsKindAndMessage: the eventual commit needs the agent's free-text
// description back, and the completion needs to know which continuation to run — both must
// survive the round trip through the store exactly as given.
func TestEnqueueGateSnapshotsKindAndMessage(t *testing.T) {
	e, ps := runEngine(t)
	if err := ps.PutAgent(store.Agent{Name: "bombur", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "sd-1"}); err != nil {
		t.Fatal(err)
	}
	r, err := e.enqueueGate("repo", "bombur", "submit", "fix the retry loop")
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "submit" || r.Message != "fix the retry loop" || r.Task != "sd-1" {
		t.Errorf("gate run = %+v, want kind=submit message=%q task=sd-1", r, "fix the retry loop")
	}
}

// TestRejectGateReturnsAgentToWorkingWithoutAPR: a failed gate must leave the agent exactly where
// an inline refusal always did — no PR, back to "working", told what to fix.
func TestRejectGateReturnsAgentToWorkingWithoutAPR(t *testing.T) {
	e, ps := runEngine(t)
	if err := ps.PutAgent(store.Agent{Name: "bombur", Role: "worker", Workspace: "."}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "sd-1", Branch: "sd-1", Phase: "gating"}); err != nil {
		t.Fatal(err)
	}
	r, err := e.enqueueGate("repo", "bombur", "submit", "my summary")
	if err != nil {
		t.Fatal(err)
	}
	deps := e.deps.(*stubDeps)
	if err := e.completeGate("repo", r, "failed", "lint: line too long"); err != nil {
		t.Fatalf("completeGate: %v", err)
	}
	if _, exists, _ := ps.GetPR("pr-sd-1"); exists {
		t.Error("a failed gate must never create a PR")
	}
	if st, _ := ps.GetState("bombur"); st.Phase != "working" || st.Task != "sd-1" {
		t.Errorf("state after a failed gate = %+v, want back to working on sd-1", st)
	}
	if len(deps.injectedText) == 0 {
		t.Fatal("the agent must be told the gate failed")
	}
	last := deps.injectedText[len(deps.injectedText)-1]
	if !strings.Contains(last, "line too long") {
		t.Errorf("the failure message should carry the violation: %q", last)
	}
}

// TestStallGateDoesNotReadAsALintFailure: a gate that timed out (or was cancelled by a hub
// restart) never found a violation, so the agent must not be told to "fix" anything, and the
// wording must not overlap with a real lint failure's.
func TestStallGateDoesNotReadAsALintFailure(t *testing.T) {
	for _, status := range []string{"timed_out", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			e, ps := runEngine(t)
			if err := ps.PutAgent(store.Agent{Name: "bombur", Role: "worker", Workspace: "."}); err != nil {
				t.Fatal(err)
			}
			if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "sd-1", Branch: "sd-1", Phase: "gating"}); err != nil {
				t.Fatal(err)
			}
			r, err := e.enqueueGate("repo", "bombur", "submit", "my summary")
			if err != nil {
				t.Fatal(err)
			}
			deps := e.deps.(*stubDeps)
			if err := e.completeGate("repo", r, status, "whatever partial output"); err != nil {
				t.Fatalf("completeGate: %v", err)
			}
			if _, exists, _ := ps.GetPR("pr-sd-1"); exists {
				t.Error("a gate that never reached a verdict must never create a PR")
			}
			if st, _ := ps.GetState("bombur"); st.Phase != "working" {
				t.Errorf("phase = %q, want working — the agent must be free to try again", st.Phase)
			}
			last := deps.injectedText[len(deps.injectedText)-1]
			if strings.Contains(last, "Lint failed") || strings.Contains(last, "violation") {
				t.Errorf("must not read as a lint failure: %q", last)
			}
			if !strings.Contains(last, "did not complete") {
				t.Errorf("should say the gate itself did not complete: %q", last)
			}
		})
	}
}

// TestExecuteGateRunUsesRepoGateNotAContainer: a gate run must never touch the container port —
// it checks the live worktree exactly as an inline gate always did. With no go.mod and no
// declared verify in the fixture, repo.Gate trivially passes; the point here is that it runs at
// all without a container runtime wired (which would error, per the exploratory-run tests).
func TestExecuteGateRunUsesRepoGateNotAContainer(t *testing.T) {
	const agent, task, branch = "bombur", "sd-1", "sd-1"
	root, _ := newWorkRepo(t, agent, branch)
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: filepath.Join(".worktrees", agent)}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Task: task, Branch: branch, Phase: "gating"}); err != nil {
		t.Fatal(err)
	}
	e := New(st, &stubDeps{root: root})
	r, err := e.enqueueGate("repo", agent, "submit", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.ExecuteRun("repo", r.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	got, _, _ := ps.GetRun(r.ID)
	if got.Status != "passed" {
		t.Fatalf("status = %q, want passed (no go.mod, no declared verify): %v", got.Status, got)
	}
	if _, exists, _ := ps.GetPR("pr-" + task); !exists {
		t.Error("a passed gate should have landed the PR")
	}
}
