package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// coauthorFixture is a repo with a coauthor resting in "collab", a worker, and that worker's open
// PR carrying an unclaimed review row. It returns the deps the engine actually holds, so a test can
// read what was delivered and to whom — reviewFixture hands back a second, unused stub.
func coauthorFixture(t *testing.T) (*Engine, *store.ProjectStore, *stubDeps) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	for _, a := range []store.Agent{
		{Name: "brokk", Role: "coauthor", Workspace: "."},
		{Name: "bombur", Role: "worker", Workspace: ".worktrees/bombur"},
	} {
		if err := ps.PutAgent(a); err != nil {
			t.Fatal(err)
		}
	}
	if err := ps.SetState(store.AgentState{Agent: "brokk", Phase: restPhase("coauthor")}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{
		ID: "pr-a", Task: "sd-1", Agent: "bombur", Branch: "sd-1", Base: "main", Status: "open",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddReview("pr-a", "review it"); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{root: root, alive: true}
	return New(st, deps), ps, deps
}

// badgeFor is the verdict recorded for author on pr, if any.
func badgeFor(t *testing.T, ps *store.ProjectStore, pr, author string) *store.Review {
	t.Helper()
	revs, err := ps.Reviews(pr)
	if err != nil {
		t.Fatalf("reviews: %v", err)
	}
	for i, r := range revs {
		if r.Author == author && r.Verdict != "" {
			return &revs[i]
		}
	}
	return nil
}

// TestACoauthorsApprovalCarriesItsName is sd-44550c's DONE WHEN for approve: a coauthor could read
// a PR, diff it and lint it, and had no way to record what it concluded. Its verdict is a real one
// — merge is human-only, so nothing lands on it — and it is attributed, which is what lets the user
// weigh "approved by the coauthor that also wrote the task" instead of the rule forbidding it.
func TestACoauthorsApprovalCarriesItsName(t *testing.T) {
	e, ps, deps := coauthorFixture(t)
	c := registry.Caller{Project: "repo", Agent: "brokk", Role: "coauthor"}

	var out strings.Builder
	if code, err := e.CmdApprove(c, []string{"pr-a"}, &out); err != nil || code != 0 {
		t.Fatalf("coauthor approve: code=%d err=%v out=%s", code, err, out.String())
	}
	if pr, _, _ := ps.GetPR("pr-a"); pr.Status != "approved" {
		t.Errorf("status = %q, want approved — a coauthor's verdict is a verdict", pr.Status)
	}
	badge := badgeFor(t, ps, "pr-a", "brokk")
	if badge == nil {
		t.Fatal("nothing recorded WHO approved — an unattributed approval is the whole thing this fixes")
	}
	if badge.Verdict != "pass" || badge.VerdictAt == "" {
		t.Errorf("badge = %+v, want a pass with a time on it", *badge)
	}
	if badge.Advisory {
		t.Error("a coauthor's approval is not advisory — that is the planner's optional second opinion")
	}
	revs, _ := ps.Reviews("pr-a")
	if n := api.ApprovalCount(revs); n != 1 {
		t.Errorf("approval count = %d, want 1: %+v", n, revs)
	}
	// It holds no review, so there is no queue to send it back to: a reviewer's verdict ends in
	// "idle" and a nudge to ask for the next one, and both would be lies told to a coauthor.
	if st, _ := ps.GetState("brokk"); st.Phase != "collab" {
		t.Errorf("phase after approving = %q, want collab — its verdict must not move it", st.Phase)
	}
	for i, name := range deps.injected {
		if name == "brokk" {
			t.Errorf("the coauthor was sent %q — nothing is coming for it to pick up", deps.injectedText[i])
		}
	}
}

// TestACoauthorsRejectionSpeaksInItsOwnName: the author weights feedback by who it is from, and a
// coauthor speaks for nobody but itself — not in the reviewer's voice, which would attribute its
// opinion to the role whose verdict actually gates the queue.
func TestACoauthorsRejectionSpeaksInItsOwnName(t *testing.T) {
	e, ps, deps := coauthorFixture(t)
	c := registry.Caller{Project: "repo", Agent: "brokk", Role: "coauthor"}

	var out strings.Builder
	if code, err := e.CmdReject(c, []string{"pr-a", "the", "gate", "is", "red"}, &out); err != nil || code != 0 {
		t.Fatalf("coauthor reject: code=%d err=%v out=%s", code, err, out.String())
	}
	pr, _, _ := ps.GetPR("pr-a")
	if pr.Status != "rejected" || !strings.Contains(pr.Feedback, "gate is red") {
		t.Errorf("PR = %q/%q, want rejected carrying the feedback", pr.Status, pr.Feedback)
	}
	badge := badgeFor(t, ps, "pr-a", "brokk")
	if badge == nil || badge.Verdict != "changes" {
		t.Fatalf("badge = %+v, want a recorded 'changes' verdict under the coauthor's name", badge)
	}
	told := -1
	for i, name := range deps.injected {
		if name == "bombur" {
			told = i
		}
	}
	if told < 0 {
		t.Fatal("the author was never told its PR was rejected")
	}
	if !strings.Contains(deps.injectedText[told], "[brokk]") {
		t.Errorf("the author was told %q — it must name who ruled", deps.injectedText[told])
	}
	if deps.delivered[told].Sender != "brokk" {
		t.Errorf("sender = %q, want brokk: provenance is stated, never read out of the wording", deps.delivered[told].Sender)
	}
	if st, _ := ps.GetState("brokk"); st.Phase != "collab" {
		t.Errorf("phase after rejecting = %q, want collab", st.Phase)
	}
}

// TestNoAgentRulesOnItsOwnCommits is the one rule that survives the softened guard (05-workflow):
// authoring the PLAN is not authoring the work, so a coauthor may rule on a PR built from a task it
// wrote — but code it wrote itself is somebody else's to judge, whatever role it holds.
func TestNoAgentRulesOnItsOwnCommits(t *testing.T) {
	for _, role := range []string{"coauthor", "reviewer", "planner"} {
		e, ps, _ := coauthorFixture(t)
		if err := ps.PutAgent(store.Agent{Name: "mine", Role: role, Workspace: ".worktrees/mine"}); err != nil {
			t.Fatal(err)
		}
		if err := ps.PutPR(store.PR{
			ID: "pr-own", Task: "sd-2", Agent: "mine", Branch: "sd-2", Base: "main", Status: "open",
		}); err != nil {
			t.Fatal(err)
		}
		c := registry.Caller{Project: "repo", Agent: "mine", Role: role}
		var out strings.Builder
		if code, _ := e.CmdApprove(c, []string{"pr-own"}, &out); code == 0 {
			t.Errorf("%s approved its own commits: %s", role, out.String())
		}
		if !strings.Contains(out.String(), "your own commits") {
			t.Errorf("%s: the refusal must say why: %q", role, out.String())
		}
		out.Reset()
		if code, _ := e.CmdReject(c, []string{"pr-own", "no"}, &out); code == 0 {
			t.Errorf("%s rejected its own commits: %s", role, out.String())
		}
		if pr, _, _ := ps.GetPR("pr-own"); pr.Status != "open" {
			t.Errorf("%s: status = %q — a refused verdict must not rewrite it", role, pr.Status)
		}
		if revs, _ := ps.Reviews("pr-own"); len(revs) != 0 {
			t.Errorf("%s: a refused verdict must record no badge: %+v", role, revs)
		}
	}
}

// TestTheQueueNeverHandsACoauthorAReview is the other half of the grant: the VERBS, not the queue.
// A reviewer pulls work and blocks on it; a coauthor reviews when the user asks. If the hub could
// hand it a review, it would be waiting on the fleet instead of on the user.
func TestTheQueueNeverHandsACoauthorAReview(t *testing.T) {
	e, ps, _ := coauthorFixture(t)

	e.AssignPendingReviews("repo") // the push path: it hands unclaimed rows to an idle reviewer
	if held, _ := ps.ReviewingPR("brokk"); held != "" {
		t.Errorf("the coauthor was handed %s — nothing may assign it a review", held)
	}
	var id int64
	var prID string
	found, err := ps.UnclaimedReview(&id, &prID)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("the review row was claimed by somebody — the only agents here are a coauthor and a worker")
	}
	// And what it is told is unchanged: the user drives it, with an open review sitting right there.
	dir, err := e.AgentDirective(context.Background(), "repo", "brokk")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if dir != DirCoauthor {
		t.Errorf("directive = %q, want the coauthor's own — never a review assignment", dir)
	}
}

// TestACoauthorAuthorsTasksLikeAPlanner: the other half of the grant. It already read the whole
// backlog and could change nothing in it; now it shapes it, and the user's approval still stands
// between a proposal and any worker claiming it — the strong role gains reach, not a gate.
func TestACoauthorAuthorsTasksLikeAPlanner(t *testing.T) {
	e, planner, ps := plannerEngine(t, "sd-seed", "")
	c := registry.Caller{Project: planner.Project, Agent: "brokk", Role: "coauthor"}

	var out strings.Builder
	if code, err := e.CmdCreateTask(c, []string{"--body", "what the user asked for", "a coauthored task"}, &out); err != nil || code != 0 {
		t.Fatalf("coauthor create-task: code=%d err=%v out=%s", code, err, out.String())
	}
	all, err := ps.AllTasks()
	if err != nil {
		t.Fatal(err)
	}
	var created string
	for _, task := range all {
		if task.ID != "sd-seed" {
			created = task.ID
		}
	}
	if created == "" {
		t.Fatal("the coauthor's task was not created")
	}
	if got, _ := ps.GetApproval(created); got != "pending" {
		t.Errorf("approval = %q, want pending — the user's gate is untouched by who proposed it", got)
	}

	if err := ps.SetApproval(created, "approved", ""); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code, err := e.CmdEditTask(c, []string{created, "sharpened", "title"}, &out); err != nil || code != 0 {
		t.Fatalf("coauthor edit-task: code=%d err=%v out=%s", code, err, out.String())
	}
	after, _, _ := ps.GetTask(created)
	if after.Title != "sharpened title" {
		t.Errorf("title = %q, want the edit applied", after.Title)
	}
	if got, _ := ps.GetApproval(created); got != "pending" {
		t.Errorf("approval = %q, want pending again — an edit is a fresh thing for the user to read", got)
	}
}
