package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestWorkerCanCommentOnItsOwnTask is the base case DONE WHEN names: a worker can record a
// finding on the task it is working, attributed to itself, and a human reading the task sees it.
func TestWorkerCanCommentOnItsOwnTask(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-1", Title: "a task", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-1", Branch: "td-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	out, code := execAs(t, h, "dvalin", "comment", "td-1", "the body is stale, needs review")
	if code != 0 {
		t.Fatalf("comment failed (%d): %s", code, out)
	}
	cs, err := ps.Comments("td-1")
	if err != nil || len(cs) != 1 {
		t.Fatalf("comments = %v, err %v, want one", cs, err)
	}
	if cs[0].Author != "dvalin" {
		t.Errorf("author = %q, want dvalin", cs[0].Author)
	}
	if cs[0].Body != "the body is stale, needs review" {
		t.Errorf("body = %q", cs[0].Body)
	}
}

// TestWorkerCanCommentOnItsHeldContainer: a container worker's context is the whole feature, not
// just its current subtask — it can leave a finding on the feature itself.
func TestWorkerCanCommentOnItsHeldContainer(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-feat", Title: "a feature", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Container: "td-feat", Branch: "td-feat", Task: "td-sub", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	out, code := execAs(t, h, "dvalin", "comment", "td-feat", "worth flagging at the feature level")
	if code != 0 {
		t.Fatalf("comment failed (%d): %s", code, out)
	}
	if cs, _ := ps.Comments("td-feat"); len(cs) != 1 {
		t.Fatalf("comments on td-feat = %v, want one", cs)
	}
}

// TestWorkerCannotCommentOnAnotherTask keeps the scope to what the worker's own state already
// grants — the isolation the rest of the surface keeps.
func TestWorkerCannotCommentOnAnotherTask(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"td-1", "td-2"} {
		if err := ps.UpsertTask(store.Task{ID: id, Title: id, Status: "open", Priority: "P1"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-1", Branch: "td-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	out, code := execAs(t, h, "dvalin", "comment", "td-2", "not mine to touch")
	if code == 0 {
		t.Fatalf("commenting on a task the worker doesn't hold should fail: %s", out)
	}
	if cs, _ := ps.Comments("td-2"); len(cs) != 0 {
		t.Errorf("comment must not have been recorded: %v", cs)
	}
}

// TestCommentIsHiddenFromAnIdleWorker: the verb belongs in the state-filtered surface — an agent
// with no assignment has nothing to comment on, so it shouldn't even see the verb.
func TestCommentIsHiddenFromAnIdleWorker(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	c := callerFor(t, h, "dvalin")
	if _, _, ok := h.registry().Resolve("comment", c); ok {
		t.Error("an idle worker should not be able to resolve `comment`")
	}
	avail := h.registry().Available(c)
	for _, cmd := range avail {
		if cmd.Name == "comment" {
			t.Error("an idle worker's surface should not list `comment`")
		}
	}
}

// TestReviewerCanCommentOnTheTaskItIsReviewing: the reviewer's scope is the PR under review, not
// an arbitrary id.
func TestReviewerCanCommentOnTheTaskItIsReviewing(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "brokkr", Role: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-1", Title: "a task", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-td-1", Task: "td-1", Agent: "dvalin", Branch: "td-1", Base: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddReview("pr-td-1", "check it"); err != nil {
		t.Fatal(err)
	}
	revs, err := ps.Reviews("pr-td-1")
	if err != nil || len(revs) == 0 {
		t.Fatalf("reviews = %v, err %v", revs, err)
	}
	if err := ps.AssignReview(revs[0].ID, "brokkr"); err != nil {
		t.Fatal(err)
	}

	out, code := execAs(t, h, "brokkr", "comment", "td-1", "the diff looks right, one nit filed here")
	if code != 0 {
		t.Fatalf("reviewer comment failed (%d): %s", code, out)
	}
	cs, _ := ps.Comments("td-1")
	if len(cs) != 1 || cs[0].Author != "brokkr" {
		t.Fatalf("comments = %v, want one authored by brokkr", cs)
	}

	// A different task, not the one under review, is refused.
	if err := ps.UpsertTask(store.Task{ID: "td-2", Title: "unrelated", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if _, code := execAs(t, h, "brokkr", "comment", "td-2", "off scope"); code == 0 {
		t.Error("a reviewer must not comment on a task it isn't reviewing")
	}
}

// TestReviewerWithNothingToReviewCannotComment: the verb is blocked, not silently a no-op.
func TestReviewerWithNothingToReviewCannotComment(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "brokkr", Role: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	c := callerFor(t, h, "brokkr")
	if _, blocked, ok := h.registry().Resolve("comment", c); ok || blocked == "" {
		t.Errorf("a reviewer with nothing to review should have `comment` blocked with a reason, ok=%v blocked=%q", ok, blocked)
	}
}

// TestPlannerCanCommentOnAnyBacklogTask: a planner already reads the whole backlog, so it may
// comment on any task in its project — not just one it holds.
func TestPlannerCanCommentOnAnyBacklogTask(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "galar", Role: "planner"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-1", Title: "someone else's task", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	out, code := execAs(t, h, "galar", "comment", "td-1", "worth splitting into two")
	if code != 0 {
		t.Fatalf("planner comment failed (%d): %s", code, out)
	}
	cs, _ := ps.Comments("td-1")
	if len(cs) != 1 || cs[0].Author != "galar" {
		t.Fatalf("comments = %v, want one authored by galar", cs)
	}

	// An unknown id still fails — a planner reads the backlog, not tasks that don't exist.
	if _, code := execAs(t, h, "galar", "comment", "td-nope", "ghost"); code == 0 {
		t.Error("commenting on an unknown task should fail")
	}
}

// TestCommentRefusesAnEmptyBody: the same guard comments.Add already enforces, reached through
// the agent verb too.
func TestCommentRefusesAnEmptyBody(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-1", Title: "a task", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-1", Branch: "td-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	out, code := execAs(t, h, "dvalin", "comment", "td-1")
	if code == 0 {
		t.Fatalf("a comment with no body should be refused: %s", out)
	}
	if !strings.Contains(out, "usage") {
		t.Errorf("want a usage message, got %q", out)
	}
}
