package store

import (
	"path/filepath"
	"testing"
)

// gateStore opens a throwaway store scoped to one project.
func gateStore(t *testing.T) *ProjectStore {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s.For("repo")
}

// TestAPassIsReusedForTheSameCommit is what keying on the commit buys: the gate costs minutes, and
// the same commit cannot have a different answer.
func TestAPassIsReusedForTheSameCommit(t *testing.T) {
	ps := gateStore(t)
	if err := ps.SetGateResult("abc123", true, "scripts/verify.sh", "all good\n"); err != nil {
		t.Fatal(err)
	}
	out, ok := ps.GatePassed("abc123", "scripts/verify.sh")
	if !ok {
		t.Fatal("a stored pass for this commit must be reusable")
	}
	if out != "all good\n" {
		t.Errorf("output = %q, want the stored gate output", out)
	}
	if _, ok := ps.GatePassed("def456", "scripts/verify.sh"); ok {
		t.Error("a commit nothing has been recorded about must not answer")
	}
}

// TestAFailureIsNotReused: a gate can fail for reasons outside the tree — a flaky test, a host under
// load — so pinning a commit to one bad run would need a human to undo. Stored, but never reused.
func TestAFailureIsNotReused(t *testing.T) {
	ps := gateStore(t)
	if err := ps.SetGateResult("abc123", false, "scripts/verify.sh", "FAIL: flaky\n"); err != nil {
		t.Fatal(err)
	}
	if _, ok := ps.GatePassed("abc123", "scripts/verify.sh"); ok {
		t.Error("a stored failure must not stand in for a fresh check")
	}
}

// TestAChangedGateIsNotReused: the verdict is about a commit AND the checks that ran over it. A
// project that re-points `verify:` asked a different question, so the old answer is not an answer.
func TestAChangedGateIsNotReused(t *testing.T) {
	ps := gateStore(t)
	if err := ps.SetGateResult("abc123", true, "scripts/verify.sh", "all good\n"); err != nil {
		t.Fatal(err)
	}
	if _, ok := ps.GatePassed("abc123", "scripts/ci.sh"); ok {
		t.Error("a pass from another verify command must not be reused")
	}
}

// TestAReGateReplacesTheVerdict: one row per commit, so a re-run cannot leave two answers about the
// same tree for a later reader to choose between.
func TestAReGateReplacesTheVerdict(t *testing.T) {
	ps := gateStore(t)
	for _, passed := range []bool{true, false} {
		if err := ps.SetGateResult("abc123", passed, "", "run\n"); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := ps.GatePassed("abc123", ""); ok {
		t.Error("the latest verdict is the verdict — a superseded pass must not answer")
	}
}

// TestEveryAskerIsRecordedOnce: a second asker joins a queued run rather than queueing another, so
// the list is how it gets told at all — and asking twice must not mean being told twice.
func TestEveryAskerIsRecordedOnce(t *testing.T) {
	ps := gateStore(t)
	for _, agent := range []string{"rune", "rune", "dvalin"} {
		if err := ps.AddRunWaiter("run-1", agent); err != nil {
			t.Fatal(err)
		}
	}
	waiters, err := ps.RunWaiters("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(waiters) != 2 || waiters[0] != "dvalin" || waiters[1] != "rune" {
		t.Errorf("waiters = %v, want each asker once", waiters)
	}
	if other, _ := ps.RunWaiters("run-2"); len(other) != 0 {
		t.Errorf("waiters on another run = %v, want none", other)
	}
}

// TestAgentWaitingOnRunCoversBothWaysToWait: a worker waits on its own run (a self-check, a queued
// `sindri run`); a reviewer waits on someone else's, having asked to be told (-> AddRunWaiter). Both
// mean the fleet's queue, not the agent, decides the next move — neither is idling on its own account.
func TestAgentWaitingOnRunCoversBothWaysToWait(t *testing.T) {
	ps := gateStore(t)
	if err := ps.PutRun(Run{ID: "run-own", Agent: "dvalin", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if waiting, err := ps.AgentWaitingOnRun("dvalin"); err != nil || !waiting {
		t.Errorf("an agent with its own queued run should read as waiting, got %v, err %v", waiting, err)
	}
	if waiting, _ := ps.AgentWaitingOnRun("nori"); waiting {
		t.Error("an agent with no run of its own and nothing waited on must not read as waiting")
	}

	if err := ps.PutRun(Run{ID: "run-pr", Agent: "system", Status: "running", Kind: "lint-pr"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.AddRunWaiter("run-pr", "ori"); err != nil {
		t.Fatal(err)
	}
	if waiting, err := ps.AgentWaitingOnRun("ori"); err != nil || !waiting {
		t.Errorf("a reviewer waiting on someone else's running gate should read as waiting, got %v, err %v", waiting, err)
	}

	// It lands: a terminal run settles nothing further, so waiting on it is over.
	if err := ps.SetRunStatus("run-own", "passed"); err != nil {
		t.Fatal(err)
	}
	if waiting, _ := ps.AgentWaitingOnRun("dvalin"); waiting {
		t.Error("a run that has already finished must not still count as waiting")
	}
}

// TestAPRLintRemembersItsCommit: the stored PR result used to carry only a timestamp, which cannot
// say which tree was checked — so no stored result could ever be trusted.
func TestAPRLintRemembersItsCommit(t *testing.T) {
	ps := gateStore(t)
	if err := ps.SetPRLint("pr-1", "abc123", "gate PASS · abc123\n"); err != nil {
		t.Fatal(err)
	}
	out, sha, ranAt := ps.GetPRLint("pr-1")
	if sha != "abc123" {
		t.Errorf("sha = %q, want the commit the result describes", sha)
	}
	if out == "" || ranAt == "" {
		t.Errorf("output = %q, ranAt = %q, want both kept alongside it", out, ranAt)
	}
}
