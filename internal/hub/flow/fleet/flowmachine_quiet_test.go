package fleet

import (
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestGatePassedSendsNoMessage is sd-c5543a: "now up for review" asks nothing of the agent — it
// waits either way — so the eventual merge or rejection is what it needs to hear, not this.
func TestGatePassedSendsNoMessage(t *testing.T) {
	const agent, task, branch = "wrk", "sd-1", "sd-1"
	root, _ := flowtest.WorkRepo(t, agent, branch)
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
	if err := ps.SetState(store.AgentState{Agent: agent, Task: task, Branch: branch, Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, ".worktrees", agent)
	if err := os.WriteFile(filepath.Join(wt, "fix.txt"), []byte("patched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := &flowtest.Hub{Root: root}
	e := newEngine(t, st, deps)
	c := registry.Caller{Project: "repo", Agent: agent, Role: "worker", Phase: "working"}
	if code, out := submitAll(t, e, c, "fix"); code != 0 {
		t.Fatalf("submit: code=%d out=%s", code, out)
	}
	runQueuedGate(t, e)
	if _, ok, _ := ps.GetPR("pr-" + task); !ok {
		t.Fatal("a passed gate should have created the PR")
	}
	if len(deps.InjectedText) != 0 {
		t.Errorf("a passed gate should tell the agent nothing — it waits either way, got: %v", deps.InjectedText)
	}
}

// TestPlainMergeSendsNoMessage is sd-c5543a's sharpest case: the task left the agent's hands the
// moment it submitted, so a merge that closes it tells the agent nothing — silence IS success, and
// nudgeIdleWorkers is what reaches it once there is new work.
func TestPlainMergeSendsNoMessage(t *testing.T) {
	const agent, task, branch = "wrk", "sd-1", "sd-1"
	root, _ := flowtest.WorkRepo(t, agent, branch)
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
	if err := ps.SetState(store.AgentState{Agent: agent, Task: task, Branch: branch, Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, ".worktrees", agent)
	if err := os.WriteFile(filepath.Join(wt, "fix.txt"), []byte("patched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := &flowtest.Hub{Root: root}
	e := newEngine(t, st, deps)
	c := registry.Caller{Project: "repo", Agent: agent, Role: "worker", Phase: "working"}
	if code, out := submitAll(t, e, c, "fix"); code != 0 {
		t.Fatalf("submit: code=%d out=%s", code, out)
	}
	runQueuedGate(t, e)
	pr, _, _ := ps.GetPR("pr-" + task)
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	before := len(deps.InjectedText)
	if _, err := e.prAct().Merge("repo", pr.ID); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	e.Look("repo", agent) // the release is the agent's own map reading a settled PR
	if got, _ := ps.GetState(agent); got.Task != "" || got.Phase != worker.Idle {
		t.Errorf("state after a closing merge = %+v, want idle with nothing held", got)
	}
	if len(deps.InjectedText) != before {
		t.Errorf("a plain closing merge should tell the agent nothing, got: %v", deps.InjectedText[before:])
	}
}

// TestAVerdictWakesWithoutClearing: a verdict frees the reviewer and tells it to carry on, and it
// leaves the session alone. Clearing is PREPARATION — it belongs to the hand-over of the next review
// (-> claimReview's own clear), where the session is reset for what it is about to do.
//
// Fired here it landed in the reviewer's own running turn, the one that called `approve`. Claude Code
// queues what is typed mid-turn, and a queued /clear discards the queue it sits in — the kickoff
// riding behind it included. vestri gave a verdict and was left cleared, idle, and told nothing.
func TestAVerdictWakesWithoutClearing(t *testing.T) {
	e, _, deps := verdictFixture(t)
	c := registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}
	if code, err := e.prAct().CmdApprove(c, []string{"pr-a"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdApprove: code=%d err=%v", code, err)
	}
	if len(deps.Cleared) != 0 {
		t.Errorf("cleared = %v, want none — a verdict is not a reason to reset a session", deps.Cleared)
	}
	// A push, so it queues behind the running turn and arrives as that turn ends.
	if len(deps.Delivered) == 0 || deps.Delivered[len(deps.Delivered)-1].Mail {
		t.Errorf("the wake must be push-only: %+v", deps.Delivered)
	}
}
