package workflow

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// userRunEngine is runEngine with an agent on the roster, since a user run can borrow one's
// workspace and the refusal for an unknown name needs a known one to contrast with.
func userRunEngine(t *testing.T) (*Engine, *store.ProjectStore) {
	t.Helper()
	e, ps := runEngine(t)
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker", Workspace: ".worktrees/eitri"}); err != nil {
		t.Fatal(err)
	}
	return e, ps
}

// TestAUserRunTargetsTheRepoCheckoutByDefault: the target only a human has. It is safe because the
// executor mounts a COPY (-> repo.MaterializeRun), so a suite writing exhaust cannot reach the tree
// the user is editing — the same isolation agent worktrees already get.
func TestAUserRunTargetsTheRepoCheckoutByDefault(t *testing.T) {
	e, ps := userRunEngine(t)
	r, err := e.ScheduleUserRun("repo", "", "make verify", "", "")
	if err != nil {
		t.Fatalf("ScheduleUserRun: %v", err)
	}
	if r.Workspace != "." {
		t.Errorf("workspace = %q, want the repo's checkout", r.Workspace)
	}
	if !api.RunFromUser(r) {
		t.Errorf("the run should be recorded as the user's, got agent %q", r.Agent)
	}
	if r.Task != "" {
		t.Errorf("a user run holds no task, got %q", r.Task)
	}
	// It is a queue entry like any other — same slot, same statuses.
	if r.Status != "queued" {
		t.Errorf("status = %q, want queued", r.Status)
	}
	if got, _, _ := ps.GetRun(r.ID); got.Command != "make verify" {
		t.Errorf("command = %q", got.Command)
	}
}

// TestAUserRunCanBorrowAnAgentWorkspace: the other target, named explicitly — never inferred from a
// working directory, since the same command means different things in different trees.
func TestAUserRunCanBorrowAnAgentWorkspace(t *testing.T) {
	e, _ := userRunEngine(t)
	r, err := e.ScheduleUserRun("repo", "eitri", "go test ./...", "", "")
	if err != nil {
		t.Fatalf("ScheduleUserRun: %v", err)
	}
	if r.Workspace != ".worktrees/eitri" {
		t.Errorf("workspace = %q, want the agent's", r.Workspace)
	}
	// An unknown one is refused rather than silently run somewhere else, and the refusal names both
	// the way to look it up and the way to mean "the repo".
	_, err = e.ScheduleUserRun("repo", "nobody", "go test ./...", "", "")
	if err == nil {
		t.Fatal("an unknown agent should be refused")
	}
	for _, want := range []string{"nobody", "agent list", "--agent"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should carry %q: %v", want, err)
		}
	}
}

// TestAUserRunGoesToTheFrontOfTheQueue: somebody is WAITING on it, while the agent behind even a
// gate run is parked and watching nothing. What that costs an agent is bounded by this run's own
// cap; a human left behind a queue of background suites is what the ordering exists to prevent.
func TestAUserRunGoesToTheFrontOfTheQueue(t *testing.T) {
	e, _ := userRunEngine(t)
	ordinary, err := e.ScheduleRun("repo", "eitri", "go build ./...", "", "")
	if err != nil {
		t.Fatal(err)
	}
	gate, err := e.putQueuedRun("repo", store.Run{Agent: "eitri", Kind: gateSubmit, Command: "make verify", Message: "done"})
	if err != nil {
		t.Fatal(err)
	}
	mine, err := e.ScheduleUserRun("repo", "", "make verify", "", "")
	if err != nil {
		t.Fatal(err)
	}

	all, err := e.store.AllRuns("queued")
	if err != nil {
		t.Fatal(err)
	}
	pos := queuePositions(all)
	if pos[mine.ID] != 1 {
		t.Errorf("the user's run should be next, got position %d", pos[mine.ID])
	}
	if pos[gate.ID] <= pos[mine.ID] || pos[ordinary.ID] <= pos[gate.ID] {
		t.Errorf("order should be user, gate, ordinary — got user=%d gate=%d ordinary=%d",
			pos[mine.ID], pos[gate.ID], pos[ordinary.ID])
	}
	// And the user can still re-order it by hand afterwards; nothing about the origin is a lock.
	if err := e.ReprioritiseRun("repo", mine.ID, "P4"); err != nil {
		t.Errorf("a user run should stay reprioritisable: %v", err)
	}
}

// TestNothingAboutAUserRunGoesStale: staleness asks whether the scheduling AGENT moved on. A user
// run has none, so a check written for agents must not drop the run a human is sitting waiting for
// on the grounds that no agent is named "user".
func TestNothingAboutAUserRunGoesStale(t *testing.T) {
	e, ps := userRunEngine(t)
	r, err := e.ScheduleUserRun("repo", "", "make verify", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if reason := e.staleReason(ps, r); reason != "" {
		t.Errorf("a user run is never stale, got %q", reason)
	}
	// The agent case is untouched: one whose agent is gone is still dropped.
	agentRun, err := e.ScheduleRun("repo", "eitri", "go test ./...", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.DeleteAgent("eitri"); err != nil {
		t.Fatal(err)
	}
	if reason := e.staleReason(ps, agentRun); reason == "" {
		t.Error("an agent run whose agent is gone should still be dropped")
	}
}

// TestAUserRunResultIsNotInjectedAnywhere: there is no session called "user". The result reaches
// the board, where the user is reading; injecting would only fill an activity log with messages
// nobody can receive.
func TestAUserRunResultIsNotInjectedAnywhere(t *testing.T) {
	e, ps := userRunEngine(t)
	deps := e.deps.(*stubDeps)
	deps.alive = true
	r, err := e.ScheduleUserRun("repo", "", "make verify", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.finishRun(ps, "repo", r, "passed", "ok\n", 0, 0, 0); err != nil {
		t.Fatalf("finishRun: %v", err)
	}
	if len(deps.injected) != 0 {
		t.Errorf("nothing should be injected for a user run, got %v", deps.injectedText)
	}
	// An agent's run still gets its summary.
	ar, err := e.ScheduleRun("repo", "eitri", "go test ./...", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.finishRun(ps, "repo", ar, "passed", "ok\n", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if len(deps.injected) != 1 || deps.injected[0] != "eitri" {
		t.Errorf("an agent's own run is still reported to it, injected: %v", deps.injected)
	}
}
