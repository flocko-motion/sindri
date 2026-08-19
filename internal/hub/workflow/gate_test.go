package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// writeFile puts content at path, so a fixture can give an agent something to gate.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// gateRepo is a worker on its own branch in a real repo — what a gate needs now that it commits
// before it checks: the store alone cannot answer "which commit is this".
func gateRepo(t *testing.T, agent, task string) (*Engine, *store.ProjectStore, string) {
	t.Helper()
	root, _ := newWorkRepo(t, agent, task)
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: filepath.Join(".worktrees", agent)}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Task: task, Branch: task, Phase: "gating"}); err != nil {
		t.Fatal(err)
	}
	return New(st, &stubDeps{root: root, projects: []store.Project{{Tag: "repo", Path: root}}}), ps, root
}

// openGate is the two steps every gate goes through, as a test helper: record the commit, then open
// the gate on it.
func openGate(t *testing.T, e *Engine, agent, kind, message string) api.Run {
	t.Helper()
	sha, err := e.gateCommit("repo", agent, message)
	if err != nil {
		t.Fatalf("gateCommit: %v", err)
	}
	r, _, err := e.gateRun("repo", agent, kind, message, sha)
	if err != nil {
		t.Fatalf("gateRun: %v", err)
	}
	return r
}

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

// TestAGateRunNamesTheCommitItChecks: the completion needs to know which continuation to run and
// what the agent said, and the verdict needs the commit it describes — a run row that named only
// the agent could not tell the store WHICH tree passed, which is why nothing was ever reusable.
func TestAGateRunNamesTheCommitItChecks(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	writeFile(t, filepath.Join(root, ".worktrees", "bombur", "new.txt"), "work")

	r := openGate(t, e, "bombur", gateSubmit, "fix the retry loop")

	if r.Kind != gateSubmit || r.Message != "fix the retry loop" || r.Task != "sd-1" {
		t.Errorf("gate run = %+v, want kind=submit message=%q task=sd-1", r, "fix the retry loop")
	}
	head, err := git.Head(filepath.Join(root, ".worktrees", "bombur"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Commit != head {
		t.Errorf("run commit = %q, want the worktree's HEAD %q", r.Commit, head)
	}
	if dirty, _ := git.HasChanges(filepath.Join(root, ".worktrees", "bombur")); dirty {
		t.Error("the gate must leave a clean tree — an uncommitted change is a tree no sha names")
	}
	if _, _, err := ps.GetRun(r.ID); err != nil {
		t.Fatalf("the run must be on record: %v", err)
	}
}

// TestASecondGateOnTheSameCommitReusesTheVerdict is the point of the whole feature: the common
// sequence is lint, change nothing, submit — and that must cost one gate, not two.
func TestASecondGateOnTheSameCommitReusesTheVerdict(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	writeFile(t, filepath.Join(root, ".worktrees", "bombur", "new.txt"), "work")

	first := openGate(t, e, "bombur", gateLint, "")
	if err := e.ExecuteRun("repo", first.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	if got, _, _ := ps.GetRun(first.ID); got.Status != "passed" {
		t.Fatalf("first gate = %q, want passed (no go.mod, no declared verify)", got.Status)
	}

	sha, err := e.gateCommit("repo", "bombur", "")
	if err != nil {
		t.Fatal(err)
	}
	second, reused, err := e.gateRun("repo", "bombur", gateSubmit, "", sha)
	if err != nil {
		t.Fatal(err)
	}
	if !reused {
		t.Fatal("a commit that already passed must not be gated again")
	}
	if got, _, _ := ps.GetRun(second.ID); got.Status != "passed" {
		t.Errorf("reused gate status = %q, want passed without executing", got.Status)
	}
	out, _ := ps.RunOutput(second.ID)
	if !strings.Contains(out, "reused") || !strings.Contains(out, shortSHA(sha)) {
		t.Errorf("a reused result must say so and name the commit, got:\n%s", out)
	}
	if _, exists, _ := ps.GetPR("pr-sd-1"); !exists {
		t.Error("the reused pass must land the PR, exactly as a fresh one does")
	}
}

// TestAChangedCommitIsGatedAgain is the other half: reuse keyed on the commit must not answer for a
// tree that has since changed, or the gate stops being a gate.
func TestAChangedCommitIsGatedAgain(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	wt := filepath.Join(root, ".worktrees", "bombur")
	writeFile(t, filepath.Join(wt, "new.txt"), "work")

	first := openGate(t, e, "bombur", gateLint, "")
	if err := e.ExecuteRun("repo", first.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	writeFile(t, filepath.Join(wt, "new.txt"), "second thoughts")

	sha, err := e.gateCommit("repo", "bombur", "")
	if err != nil {
		t.Fatal(err)
	}
	if sha == first.Commit {
		t.Fatal("a changed tree must produce a different commit")
	}
	if _, reused, err := e.gateRun("repo", "bombur", gateSubmit, "", sha); err != nil || reused {
		t.Fatalf("reused = %v (err %v), want a fresh gate for a commit nothing has said anything about", reused, err)
	}
	if _, exists, _ := ps.GetPR("pr-sd-1"); exists {
		t.Error("no PR may exist before the gate on the new commit has run")
	}
}

// TestRejectGateReturnsAgentToWorkingWithoutAPR: a failed gate must leave the agent exactly where
// an inline refusal always did — no PR, back to "working", told what to fix.
func TestRejectGateReturnsAgentToWorkingWithoutAPR(t *testing.T) {
	e, ps, _ := gateRepo(t, "bombur", "sd-1")
	r := openGate(t, e, "bombur", gateSubmit, "my summary")
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
			e, ps, _ := gateRepo(t, "bombur", "sd-1")
			r := openGate(t, e, "bombur", gateSubmit, "my summary")
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
	const agent, task = "bombur", "sd-1"
	e, ps, _ := gateRepo(t, agent, task)
	r := openGate(t, e, agent, gateSubmit, "")
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
