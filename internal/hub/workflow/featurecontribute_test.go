package workflow

import (
	"io"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestFeatureWorkerContributesTheBranch: a feature is long-running by design — several subtasks
// accumulating on a standing branch — so the window in which nobody else can use the work is LONGER
// there, not shorter. The verb was withheld inside a feature on the reasoning that checkpoint covered
// it, but checkpoint records onto the branch and nothing reaches the reference branch until a PR
// merges. What goes up is the feature, named for the feature, and the worker keeps it across the
// landing.
func TestFeatureWorkerContributesTheBranch(t *testing.T) {
	e, ps, c := featureWorker(t, true) // a subtask still open: mid-feature
	var out strings.Builder
	if code, err := e.CmdContribute(c, []string{"the parser is usable now"}, &out); err != nil || code != 0 {
		t.Fatalf("CmdContribute: code=%d err=%v out=%s", code, err, out.String())
	}

	// Named for the FEATURE: the branch carries every checkpointed subtask, so a PR named for the
	// subtask in hand would misdescribe what is in it.
	pr, ok, _ := ps.GetPR("pr-td-EPIC")
	if !ok {
		t.Fatal("contributing inside a feature should put the feature branch up as pr-td-EPIC")
	}
	if pr.Task != "td-EPIC" || pr.Branch != "td-EPIC" {
		t.Errorf("PR = {task:%q branch:%q}, want the feature on both", pr.Task, pr.Branch)
	}
	// Interim, so nothing reads the merge as the feature having landed.
	if pr.Kind != "interim" || pr.Status != "open" {
		t.Errorf("PR = {kind:%q status:%q}, want an open interim one", pr.Kind, pr.Status)
	}
	if revs, _ := ps.Reviews(pr.ID); len(revs) != 0 {
		t.Errorf("an interim PR is user-gated, got %d review(s)", len(revs))
	}
	// Parked on the verdict, still holding the feature — the association that a missing Container
	// silently dropped, which reads as an unrelated bug days later.
	st, _ := ps.GetState("dain")
	if st.Phase != "submitted" || st.Container != "td-EPIC" {
		t.Errorf("state = {phase:%q container:%q}, want it waiting and still on the feature", st.Phase, st.Container)
	}
	if !strings.Contains(out.String(), "td-EPIC") {
		t.Errorf("the reply should name what went up: %s", out.String())
	}
}

// TestFeatureContributionMergeKeepsTheFeature: the merge lands the work and the worker carries on
// with the same feature — the whole point of an interim landing. Closing td-EPIC here would finish a
// feature that still has subtasks to do.
func TestFeatureContributionMergeKeepsTheFeature(t *testing.T) {
	e, ps, c := featureWorker(t, true)
	if code, err := e.CmdContribute(c, nil, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdContribute: code=%d err=%v", code, err)
	}
	pr, _, _ := ps.GetPR("pr-td-EPIC")
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Merge("repo", pr.ID); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	st, _ := ps.GetState("dain")
	if st.Container != "td-EPIC" || st.Branch != "td-EPIC" || st.Phase != "working" {
		t.Errorf("state = {container:%q branch:%q phase:%q}, want it back at work on the feature",
			st.Container, st.Branch, st.Phase)
	}
	feature, _, _ := ps.GetTask("td-EPIC")
	if feature.Status != "open" {
		t.Errorf("feature status = %q — an instalment of a feature must not finish it", feature.Status)
	}
	// And it is still claimable work: a merged interim PR is not the feature having landed.
	if err := ps.SetState(store.AgentState{Agent: "dain", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	open, err := ps.OpenContainers()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].ID != "td-EPIC" {
		t.Errorf("open containers = %v, want td-EPIC still on offer", open)
	}
}
