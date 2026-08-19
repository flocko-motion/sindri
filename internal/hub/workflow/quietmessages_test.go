package workflow

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestGatePassedSendsNoMessage is sd-c5543a: "now up for review" asks nothing of the agent — it
// waits either way — so the eventual merge or rejection is what it needs to hear, not this.
func TestGatePassedSendsNoMessage(t *testing.T) {
	const agent, task, branch = "wrk", "sd-1", "sd-1"
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
	if err := ps.UpsertTask(store.Task{ID: task, Title: "fix it", Type: "bug", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Task: task, Branch: branch, Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, ".worktrees", agent)
	if err := os.WriteFile(filepath.Join(wt, "fix.txt"), []byte("patched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{root: root}
	e := New(st, deps)
	c := registry.Caller{Project: "repo", Agent: agent, Role: "worker", Phase: "working"}
	if code, err := e.CmdSubmit(c, []string{"fix"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdSubmit: code=%d err=%v", code, err)
	}
	runQueuedGate(t, e)
	if _, ok, _ := ps.GetPR("pr-" + task); !ok {
		t.Fatal("a passed gate should have created the PR")
	}
	if len(deps.injectedText) != 0 {
		t.Errorf("a passed gate should tell the agent nothing — it waits either way, got: %v", deps.injectedText)
	}
}

// TestPlainMergeSendsNoMessage is sd-c5543a's sharpest case: the task left the agent's hands the
// moment it submitted, so a merge that closes it tells the agent nothing — silence IS success, and
// nudgeIdleWorkers is what reaches it once there is new work.
func TestPlainMergeSendsNoMessage(t *testing.T) {
	const agent, task, branch = "wrk", "sd-1", "sd-1"
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
	if err := ps.UpsertTask(store.Task{ID: task, Title: "fix it", Type: "bug", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Task: task, Branch: branch, Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, ".worktrees", agent)
	if err := os.WriteFile(filepath.Join(wt, "fix.txt"), []byte("patched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{root: root}
	e := New(st, deps)
	c := registry.Caller{Project: "repo", Agent: agent, Role: "worker", Phase: "working"}
	if code, err := e.CmdSubmit(c, []string{"fix"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdSubmit: code=%d err=%v", code, err)
	}
	runQueuedGate(t, e)
	pr, _, _ := ps.GetPR("pr-" + task)
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	before := len(deps.injectedText)
	if _, err := e.Merge("repo", pr.ID); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got, _ := ps.GetState(agent); got.Task != "" || got.Phase != "idle" {
		t.Errorf("state after a closing merge = %+v, want idle with nothing held", got)
	}
	if len(deps.injectedText) != before {
		t.Errorf("a plain closing merge should tell the agent nothing, got: %v", deps.injectedText[before:])
	}
}

// TestMilestoneAndInterimMergesArePushOnly: unlike a closing merge, these resume work the agent
// still holds — a real wake is needed — but the content is exactly what its own directive already
// says, so it must not become a permanent mailbox entry nobody reads.
func TestMilestoneAndInterimMergesArePushOnly(t *testing.T) {
	t.Run("milestone", func(t *testing.T) {
		e, ps, c, deps := featureWorker(t, true)
		if code, err := e.CmdContribute(c, nil, io.Discard); err != nil || code != 0 {
			t.Fatalf("CmdContribute: code=%d err=%v", code, err)
		}
		runQueuedGate(t, e)
		pr, _, _ := ps.GetPR("pr-td-EPIC")
		pr.Status = "approved"
		if err := ps.PutPR(pr); err != nil {
			t.Fatal(err)
		}
		before := len(deps.delivered)
		if _, err := e.Merge("repo", pr.ID); err != nil {
			t.Fatalf("Merge: %v", err)
		}
		assertLastPushOnly(t, deps, before, "td-EPIC")
	})
	t.Run("interim", func(t *testing.T) {
		e, ps, c, deps := leafWorker(t, "working")
		if code, err := e.CmdContribute(c, nil, io.Discard); err != nil || code != 0 {
			t.Fatalf("CmdContribute: code=%d err=%v", code, err)
		}
		runQueuedGate(t, e)
		pr, _, _ := ps.GetPR("pr-td-LEAF")
		pr.Status = "approved"
		if err := ps.PutPR(pr); err != nil {
			t.Fatal(err)
		}
		before := len(deps.delivered)
		if _, err := e.Merge("repo", pr.ID); err != nil {
			t.Fatalf("Merge: %v", err)
		}
		assertLastPushOnly(t, deps, before, "td-LEAF")
	})
}

// TestVerdictClearsTheReviewer: a verdict is the reviewer's own leaf boundary, and its session must
// carry nothing from this review into the next — so approving fires a clear rather than leaving a
// mailbox entry behind.
func TestVerdictClearsTheReviewer(t *testing.T) {
	e, _, deps := verdictFixture(t)
	c := registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}
	if code, err := e.CmdApprove(c, []string{"pr-a"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdApprove: code=%d err=%v", code, err)
	}
	if len(deps.cleared) != 1 || deps.cleared[0] != "fili" {
		t.Errorf("cleared = %v, want exactly one FireClear(fili)", deps.cleared)
	}
}

func assertLastPushOnly(t *testing.T, deps *stubDeps, before int, wantSubstring string) {
	t.Helper()
	if len(deps.delivered) <= before {
		t.Fatal("the merge should have delivered a wake to resume the agent")
	}
	last := deps.delivered[len(deps.delivered)-1]
	if last.Mail || !last.Push {
		t.Errorf("resuming a held task/feature must be push-only, got %+v", last)
	}
	if !strings.Contains(deps.injectedText[len(deps.injectedText)-1], wantSubstring) {
		t.Errorf("the wake should name what it resumes (%q): %s", wantSubstring, deps.injectedText[len(deps.injectedText)-1])
	}
}
