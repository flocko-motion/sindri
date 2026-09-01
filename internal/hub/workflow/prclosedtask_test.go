package workflow

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestAPRWhoseTaskClosedIsScrapped is hepti's row. A rejected PR is still LIVE (a resubmission
// clears it), so one whose task closed underneath had somewhere to land in every reader's eyes and
// nowhere to land in fact. AwaitingPR carried an exception for it; the board's own openPRFor did
// not, so the agent showed a PR on a task closed a week earlier while the hold rule said otherwise.
// Settled at the source instead, so both agree without either restating the rule.
func TestAPRWhoseTaskClosedIsScrapped(t *testing.T) {
	e, ps, _ := ownedEngine(t, "open")
	if err := ps.UpsertTask(store.Task{ID: "sd-1", Title: "done with", Status: "closed"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-sd-1", Task: "sd-1", Agent: "hepti", Status: "rejected", Kind: "final"}); err != nil {
		t.Fatal(err)
	}

	if err := e.ReconcileTasks("proj"); err != nil {
		t.Fatalf("ReconcileTasks: %v", err)
	}
	pr, ok, _ := ps.GetPR("pr-sd-1")
	if !ok {
		t.Fatal("the PR is gone — it should be settled, not deleted")
	}
	if pr.Status != "scrapped" {
		t.Errorf("a PR with nowhere to land should be scrapped, got %q", pr.Status)
	}
	if pr.Feedback == "" {
		t.Error("the PR says nothing about why it was settled — its author reads this")
	}
	// And the hold goes with it, which is the whole point: the agent is free.
	if held, _, _ := ps.AwaitingPR("hepti"); held != "" {
		t.Errorf("the agent is still held by %s", held)
	}
}

// TestALivePROnAnOpenTaskIsLeftAlone: the sweep runs on every task listing, so a rule that reached
// one row too far would settle work that is genuinely waiting for a verdict.
func TestALivePROnAnOpenTaskIsLeftAlone(t *testing.T) {
	e, ps, _ := ownedEngine(t, "open")
	if err := ps.UpsertTask(store.Task{ID: "sd-2", Title: "still going", Status: "in_progress"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-sd-2", Task: "sd-2", Agent: "hepti", Status: "rejected", Kind: "final"}); err != nil {
		t.Fatal(err)
	}
	if err := e.ReconcileTasks("proj"); err != nil {
		t.Fatal(err)
	}
	if pr, _, _ := ps.GetPR("pr-sd-2"); pr.Status != "rejected" {
		t.Errorf("a rejected PR on an open task is the ordinary rework loop, got %q", pr.Status)
	}
}
