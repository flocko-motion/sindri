package pr

import (
	"strings"
	"testing"
)

// TestFeatureWorkerContributesTheBranch: a feature is long-running by design — several subtasks
// accumulating on a standing branch — so the window in which nobody else can use the work is LONGER
// there, not shorter. The verb was withheld inside a feature on the reasoning that checkpoint covered
// it, but checkpoint records onto the branch and nothing reaches the reference branch until a PR
// merges. What goes up is the feature, named for the feature, and the worker keeps it across the
// landing.
func TestFeatureWorkerContributesTheBranch(t *testing.T) {
	a, ps, c, deps := featureWorker(t, true) // a subtask still open: mid-feature
	var out strings.Builder
	if code, err := a.CmdContribute(c, []string{"the parser is usable now"}, &out); err != nil || code != 0 {
		t.Fatalf("CmdContribute: code=%d err=%v out=%s", code, err, out.String())
	}
	if !strings.Contains(out.String(), "queued") {
		t.Errorf("the immediate reply should say the gate is queued, nothing has landed yet: %s", out.String())
	}
	runQueuedGate(t, a)

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
	// Still on the feature — the association a missing Container silently dropped, which reads as an
	// unrelated bug days later. Where the milestone leaves it is its own map's: a pull request of its
	// own is out, and that is the branch it would carry on (-> worker/working's cond.OwnPROpen).
	st, _ := ps.GetState("dain")
	if st.Container != "td-EPIC" {
		t.Errorf("container = %q, want the feature still held", st.Container)
	}
	// What went up is named in the message injected once the gate passes — the immediate reply
	// could not have named it, since nothing had landed yet when it was sent.
	if len(deps.InjectedText) == 0 || !strings.Contains(deps.InjectedText[len(deps.InjectedText)-1], "td-EPIC") {
		t.Errorf("the agent should be told what went up: %v", deps.InjectedText)
	}
}
