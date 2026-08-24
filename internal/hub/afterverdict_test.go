package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// ruledReviewer registers a reviewer and gives it an assigned review of each pr, over a task named
// after it — the state assignReview leaves behind, which ruleOn then completes.
func ruledReviewer(t *testing.T, h *Hub, reviewer string, prs ...string) {
	t.Helper()
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: reviewer, Role: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	for _, pr := range prs {
		task := "td-" + strings.TrimPrefix(pr, "pr-")
		if err := ps.UpsertTask(store.Task{ID: task, Title: "a task", Status: "open", Priority: "P1"}); err != nil {
			t.Fatal(err)
		}
		if err := ps.PutPR(store.PR{ID: pr, Task: task, Agent: "dvalin", Branch: task, Base: "main", Status: "open"}); err != nil {
			t.Fatal(err)
		}
		id, err := ps.AddReview(pr, "check it")
		if err != nil {
			t.Fatal(err)
		}
		if err := ps.AssignReview(id, reviewer); err != nil {
			t.Fatal(err)
		}
	}
}

// ruleOn records reviewer's verdict on its oldest unverdicted review of pr, as CmdApprove's
// completeReview does — the write that used to shut the comment verb.
func ruleOn(t *testing.T, h *Hub, reviewer, pr string) {
	t.Helper()
	ps := h.store.For(testProject)
	revs, err := ps.Reviews(pr)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range revs {
		if r.Author == reviewer && r.Verdict == "" {
			if err := ps.RecordVerdict(r.ID, "changes", "not yet"); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("%s holds no open review of %s", reviewer, pr)
}

// TestAReviewerStillCommentsAfterItsVerdict (sd-fa5d6d) is the cliff: the gate asked for the review
// it HOLDS, and recording the verdict is what ends that — so the verb shut on the same call that
// submitted the rejection, at the exact moment the reviewer acquired afterthoughts. Its only
// remaining channel was mail, which nothing on the task ever leads a later reader to.
func TestAReviewerStillCommentsAfterItsVerdict(t *testing.T) {
	h := newHub(t)
	ruledReviewer(t, h, "brokkr", "pr-1")
	ruleOn(t, h, "brokkr", "pr-1")

	out, code := execAs(t, h, "brokkr", "comment", "one more thought about that design choice")
	if code != 0 {
		t.Fatalf("a reviewer that has ruled must still be able to comment (%d): %s", code, out)
	}
	cs, _ := h.store.For(testProject).Comments("td-1")
	if len(cs) != 1 || cs[0].Author != "brokkr" {
		t.Fatalf("comments on td-1 = %v, want one from brokkr — on the TASK, where a later reader finds it", cs)
	}
	// And the verb is offered, not merely functional: a blocked verb is one an agent never tries.
	if reason := commentBlockedFor(t, h, "brokkr"); reason != "" {
		t.Errorf("comment is held back from a reviewer that has ruled: %q", reason)
	}
}

// TestTheHeldReviewOutranksAnOlderVerdict: an id-less comment means the newest thing in reach, which
// is the review in hand while there is one — not the PR it ruled on last week.
func TestTheHeldReviewOutranksAnOlderVerdict(t *testing.T) {
	h := newHub(t)
	ruledReviewer(t, h, "brokkr", "pr-1", "pr-2")
	ruleOn(t, h, "brokkr", "pr-1") // ruled, then handed a second review it still holds

	if out, code := execAs(t, h, "brokkr", "comment", "about the one in hand"); code != 0 {
		t.Fatalf("comment failed (%d): %s", code, out)
	}
	ps := h.store.For(testProject)
	if cs, _ := ps.Comments("td-2"); len(cs) != 1 {
		t.Errorf("the bare comment should land on the held review's task, td-2 has %v", cs)
	}
	if cs, _ := ps.Comments("td-1"); len(cs) != 0 {
		t.Errorf("the older verdict's task took the comment: td-1 has %v", cs)
	}
	// The older one stays reachable BY NAME, which is the point of widening rather than moving.
	if out, code := execAs(t, h, "brokkr", "comment", "td-1", "and one about the earlier PR"); code != 0 {
		t.Fatalf("naming a task it has ruled on failed (%d): %s", code, out)
	}
	if cs, _ := ps.Comments("td-1"); len(cs) != 1 {
		t.Errorf("naming td-1 must reach it; it has %v", cs)
	}
}

// TestAReviewerStillCannotWanderTheBacklog: the gate exists so a reviewer cannot comment on work it
// knows nothing about, and having ruled is the proof it read something. That test is unchanged.
func TestAReviewerStillCannotWanderTheBacklog(t *testing.T) {
	h := newHub(t)
	ruledReviewer(t, h, "brokkr", "pr-1")
	ruleOn(t, h, "brokkr", "pr-1")
	ps := h.store.For(testProject)
	if err := ps.UpsertTask(store.Task{ID: "td-elsewhere", Title: "somebody else's", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}

	out, code := execAs(t, h, "brokkr", "comment", "td-elsewhere", "an opinion nobody asked for")
	if code == 0 {
		t.Fatalf("a reviewer must not comment on a task it has never reviewed: %s", out)
	}
	if cs, _ := ps.Comments("td-elsewhere"); len(cs) != 0 {
		t.Errorf("the comment landed anyway: %v", cs)
	}
	// The refusal names what IS reachable — a wall with no way forward is the house's own complaint.
	if !strings.Contains(out, "td-1") {
		t.Errorf("the refusal should name the task it can comment on, got %q", out)
	}
}

// TestAReviewerWithNoVerdictYetIsToldWhatWouldReachOne: nothing held and nothing ruled on is the one
// case that still refuses, and it says how the verb opens rather than only that it is shut.
func TestAReviewerWithNoVerdictYetIsToldWhatWouldReachOne(t *testing.T) {
	h := newHub(t)
	if err := h.store.For(testProject).PutAgent(store.Agent{Name: "brokkr", Role: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	reason := commentBlockedFor(t, h, "brokkr")
	if reason == "" {
		t.Fatal("a reviewer that has never reviewed anything has no task to comment on")
	}
	for _, want := range []string{"approve", "reject"} {
		if !strings.Contains(reason, want) {
			t.Errorf("the refusal should name what opens the verb (%q), got %q", want, reason)
		}
	}
}

// commentBlockedFor is why the comment verb is held back from an agent, "" when it is offered — read
// off the same surface the agent sees, so the gate and the verb cannot be tested apart.
func commentBlockedFor(t *testing.T, h *Hub, agent string) string {
	t.Helper()
	cmds, err := h.AgentCommands(testProject, agent)
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range cmds {
		if cmd.Name == "comment" {
			return cmd.Unavailable
		}
	}
	t.Fatalf("%s was not offered `comment` at all", agent)
	return ""
}
