package workflow

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestGlobalReviewerFollowsAReviewPastAssignment is the end-to-end regression a review found: a
// pooled reviewer's own project (GlobalProject) is never the PR's project, so every verb after
// assignment — asking for its own directive, approving — has to resolve the PR's actual project
// rather than trusting its own, or the reviewer is told it holds nothing while holding exactly one.
func TestGlobalReviewerFollowsAReviewPastAssignment(t *testing.T) {
	st, ps := poolFixture(t)
	if err := st.For(GlobalProject).PutAgent(store.Agent{Name: "ori", Role: "reviewer", Workspace: "ori"}); err != nil {
		t.Fatal(err)
	}
	// KnownProjects must list "repo": reviewingPR's fleet-wide scan for a GlobalProject reviewer
	// only checks the projects the fleet knows about, exactly like AssignPendingReviews's own caller.
	e := New(st, &stubDeps{root: t.TempDir(), alive: true, projects: []store.Project{{Tag: "repo"}}})

	e.AssignPendingReviews("repo")
	if held, _ := ps.ReviewingPR("ori"); held != "pr-1" {
		t.Fatalf("setup: ori should hold pr-1, got %q", held)
	}

	// Asking for its own directive — from GlobalProject, its own project — must find the review it
	// holds rather than answering DirNoReviews.
	dir, ok, err := e.reviewDirective(t.Context(), GlobalProject, "ori")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("reviewDirective should answer")
	}
	if dir == DirNoReviews {
		t.Fatal("reviewDirective answered DirNoReviews while ori holds pr-1")
	}
	if !strings.Contains(dir, "pr-1") {
		t.Errorf("reviewDirective = %q, want it to name pr-1", dir)
	}

	// Approving, as ori itself (Caller.Project is ori's own home, GlobalProject — never pr-1's
	// project) must still find and settle pr-1.
	deps := &stubDeps{root: t.TempDir(), alive: true}
	e2 := New(st, deps)
	c := registry.Caller{Project: GlobalProject, Agent: "ori", Role: "reviewer"}
	if code, err := e2.CmdApprove(c, []string{"pr-1"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdApprove: code=%d err=%v", code, err)
	}
	pr, _, _ := ps.GetPR("pr-1")
	if pr.Status != "approved" {
		t.Errorf("pr-1 status = %q, want approved", pr.Status)
	}
	if held, _ := ps.ReviewingPR("ori"); held != "" {
		t.Errorf("ori should no longer hold a verdict-less review, got %q", held)
	}
	revs, _ := ps.Reviews("pr-1")
	var stamped bool
	for _, r := range revs {
		if r.Author == "ori" && r.Verdict == "pass" {
			stamped = true
		}
	}
	if !stamped {
		t.Error("no recorded pass verdict from ori found on pr-1's review row")
	}
	// No clear. Clearing is PREPARATION and belongs to the next hand-over (-> claimReview's
	// compactIfDue): fired here it lands in the reviewer's own running turn — the one that called
	// approve — where a queued /clear discards the kickoff queued behind it and leaves it idle.
	if len(deps.cleared) != 0 {
		t.Errorf("cleared = %v, want none — a session is prepared for what it is about to do", deps.cleared)
	}
}

// TestGlobalReviewerCanRejectAcrossProjects is the same regression on the other verdict.
func TestGlobalReviewerCanRejectAcrossProjects(t *testing.T) {
	st, ps := poolFixture(t)
	if err := st.For(GlobalProject).PutAgent(store.Agent{Name: "ori", Role: "reviewer", Workspace: "ori"}); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{root: t.TempDir(), alive: true}
	e := New(st, deps)
	e.AssignPendingReviews("repo")

	c := registry.Caller{Project: GlobalProject, Agent: "ori", Role: "reviewer"}
	if code, err := e.CmdReject(c, []string{"pr-1", "needs", "another", "pass"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdReject: code=%d err=%v", code, err)
	}
	pr, _, _ := ps.GetPR("pr-1")
	if pr.Status != "rejected" {
		t.Errorf("pr-1 status = %q, want rejected", pr.Status)
	}
	if len(deps.cleared) != 0 {
		t.Errorf("cleared = %v, want none — a verdict is not a reason to reset a session", deps.cleared)
	}
}

// TestGlobalReviewerCanShowThePRItHolds: CmdShowPR must find pr-1 under its own project once ori
// actually holds it, even though the caller's project is GlobalProject — the diff needs a real repo
// and fails harmlessly here, but the metadata printed before that is what proves the PR was found.
func TestGlobalReviewerCanShowThePRItHolds(t *testing.T) {
	st, ps := poolFixture(t)
	if err := st.For(GlobalProject).PutAgent(store.Agent{Name: "ori", Role: "reviewer", Workspace: "ori"}); err != nil {
		t.Fatal(err)
	}
	e := New(st, &stubDeps{root: t.TempDir(), alive: true})
	e.AssignPendingReviews("repo")
	if held, _ := ps.ReviewingPR("ori"); held != "pr-1" {
		t.Fatalf("setup: ori should hold pr-1, got %q", held)
	}
	c := registry.Caller{Project: GlobalProject, Agent: "ori", Role: "reviewer"}

	var out bytes.Buffer
	_, _ = e.CmdShowPR(c, []string{"pr-1"}, &out)
	if !strings.Contains(out.String(), "pr-1") {
		t.Errorf("show did not find pr-1, which ori holds:\n%s", out.String())
	}
}

// TestShowIsScopedUnlessTheCallerHoldsTheNamedPR is the isolation regression a review found:
// widening PRProject unconditionally for show/lint reached every agent in every repo, not only a
// pooled reviewer holding the PR it names. An unrelated agent naming a foreign PR id must be
// refused, exactly as it was before a fleet-wide reviewer pool existed.
func TestShowIsScopedUnlessTheCallerHoldsTheNamedPR(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	other := st.For("other-repo")
	if err := other.PutPR(store.PR{ID: "pr-9", Task: "td-9", Agent: "dain", Branch: "sd-9", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := st.For("mine").PutAgent(store.Agent{Name: "eitri", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	e := New(st, &stubDeps{root: t.TempDir(), alive: true})
	c := registry.Caller{Project: "mine", Agent: "eitri", Role: "worker"}

	var out bytes.Buffer
	code, err := e.CmdShowPR(c, []string{"pr-9"}, &out)
	// Refused on `out` with a NIL error: that is the convention for an agent-actionable outcome, and a
	// returned error means a hub-internal fault — which now escalates the agent (-> AgentExec). Demanding
	// one here would escalate every worker that mistypes a PR id.
	if err != nil {
		t.Fatalf("a refusal must not read as a hub fault: %v", err)
	}
	if code == 0 {
		t.Fatalf("a worker in another repo should be refused pr-9, got code=%d: %s", code, out.String())
	}
	// Echoing back the id the caller itself typed is not a leak; the PR's own data is.
	for _, secret := range []string{"td-9", "dain", "sd-9"} {
		if strings.Contains(out.String(), secret) {
			t.Errorf("pr-9's own data (%q) leaked to an unrelated caller: %s", secret, out.String())
		}
	}
}

// foreignPRFixture is one PR in another-repo and one reviewer in mine, neither ever brought
// together — the shape both write-side isolation tests need.
func foreignPRFixture(t *testing.T) (*store.Store, *store.ProjectStore, registry.Caller) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	other := st.For("other-repo")
	if err := other.PutPR(store.PR{ID: "pr-9", Task: "td-9", Agent: "dain", Branch: "sd-9", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := st.For("mine").PutAgent(store.Agent{Name: "fili", Role: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	return st, other, registry.Caller{Project: "mine", Agent: "fili", Role: "reviewer"}
}

// TestApproveIsScopedUnlessTheCallerHoldsTheNamedPR is the write-side isolation regression: openPR
// widened unconditionally via PRProject, so a reviewer naming a foreign PR id it never held could
// approve it. It must be refused, and the PR must be untouched.
func TestApproveIsScopedUnlessTheCallerHoldsTheNamedPR(t *testing.T) {
	st, other, c := foreignPRFixture(t)
	e := New(st, &stubDeps{root: t.TempDir(), alive: true})

	var out bytes.Buffer
	code, err := e.CmdApprove(c, []string{"pr-9"}, &out)
	if err != nil {
		t.Fatalf("the refusal came back as a hub fault, which escalates the caller: %v", err)
	}
	if code == 0 {
		t.Fatalf("a reviewer holding nothing in other-repo should be refused pr-9, got code=%d", code)
	}
	if !strings.Contains(out.String(), "No PR pr-9") {
		t.Errorf("the caller must be told the id is out of its reach, got %q", out.String())
	}
	if pr, _, _ := other.GetPR("pr-9"); pr.Status != "open" {
		t.Errorf("pr-9's status changed to %q from an unrelated caller's approve attempt", pr.Status)
	}
}

// TestRejectIsScopedUnlessTheCallerHoldsTheNamedPR is the same regression on the other verdict.
func TestRejectIsScopedUnlessTheCallerHoldsTheNamedPR(t *testing.T) {
	st, other, c := foreignPRFixture(t)
	e := New(st, &stubDeps{root: t.TempDir(), alive: true})

	var out bytes.Buffer
	code, err := e.CmdReject(c, []string{"pr-9", "no"}, &out)
	if err != nil {
		t.Fatalf("the refusal came back as a hub fault, which escalates the caller: %v", err)
	}
	if code == 0 {
		t.Fatalf("a reviewer holding nothing in other-repo should be refused pr-9, got code=%d", code)
	}
	if !strings.Contains(out.String(), "No PR pr-9") {
		t.Errorf("the caller must be told the id is out of its reach, got %q", out.String())
	}
	if pr, _, _ := other.GetPR("pr-9"); pr.Status != "open" {
		t.Errorf("pr-9's status changed to %q from an unrelated caller's reject attempt", pr.Status)
	}
}

// TestRequestReviewReachesThePoolDirectly is the submit-path regression: a project with no reviewer
// of its own must be handed to a GlobalProject reviewer at request time (freeReviewer), not only
// once the periodic AssignPendingReviews tick runs.
func TestRequestReviewReachesThePoolDirectly(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "td-1", Agent: "bombur", Branch: "sd-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := st.For(GlobalProject).PutAgent(store.Agent{Name: "ori", Role: "reviewer", Workspace: "ori"}); err != nil {
		t.Fatal(err)
	}
	e := New(st, &stubDeps{root: t.TempDir(), alive: true})

	if err := e.RequestReview("repo", "pr-1", "look"); err != nil {
		t.Fatal(err)
	}
	if held, _ := ps.ReviewingPR("ori"); held != "pr-1" {
		t.Errorf("ori should hold pr-1 immediately from RequestReview, got %q — the pool must not need the tick", held)
	}
}

// TestGlobalReviewerReadsTheTasksOfTheProjectItReviewsFor is the blind-review regression vestri
// reported: `sindri task <id>` and `task list` read the caller's own project, and a pooled reviewer's
// own project holds no tasks at all. So it answered "0 active tasks" and "no such task" for the very
// task its PR implements, and the review was made against the diff alone — silently, since nothing
// about that reads as a failure. A reviewer works for the project whose PR it holds.
func TestGlobalReviewerReadsTheTasksOfTheProjectItReviewsFor(t *testing.T) {
	st, ps := poolFixture(t)
	if err := st.For(GlobalProject).PutAgent(store.Agent{Name: "ori", Role: "reviewer", Workspace: "ori"}); err != nil {
		t.Fatal(err)
	}
	e := New(st, &stubDeps{root: t.TempDir(), alive: true})
	e.AssignPendingReviews("repo")
	if held, _ := ps.ReviewingPR("ori"); held != "pr-1" {
		t.Fatalf("setup: ori should hold pr-1, got %q", held)
	}
	c := registry.Caller{Project: GlobalProject, Agent: "ori", Role: "reviewer"}

	if home := e.taskHome(c); home != "repo" {
		t.Errorf("taskHome while holding repo's pr-1 = %q, want \"repo\"", home)
	}
	// An unheld reviewer borrows nothing: there is no project to read, and inventing one would let it
	// read a backlog it was never handed anything from.
	idle := registry.Caller{Project: GlobalProject, Agent: "nobody", Role: "reviewer"}
	if home := e.taskHome(idle); home != GlobalProject {
		t.Errorf("taskHome holding no review = %q, want %q", home, GlobalProject)
	}
	// A project-bound caller is untouched, whatever any reviewer holds.
	local := registry.Caller{Project: "repo", Agent: "dvalin", Role: "worker"}
	if home := e.taskHome(local); home != "repo" {
		t.Errorf("taskHome for a project-bound caller = %q, want its own project", home)
	}
}

// TestAnUnknownTaskIsAnsweredNotEscalated: `sindri task <id>` for an id the caller's project does
// not carry is an ANSWER. Returned as an error it reaches AgentExec as a hub-internal failure,
// which auto-escalates — and balin, a pooled reviewer between reviews, was stranded on exactly that
// for asking about a task in the repo it had just been reviewing for.
func TestAnUnknownTaskIsAnsweredNotEscalated(t *testing.T) {
	st, _ := poolFixture(t)
	if err := st.For(GlobalProject).PutAgent(store.Agent{Name: "balin", Role: "reviewer", Workspace: "balin"}); err != nil {
		t.Fatal(err)
	}
	e := New(st, &stubDeps{root: t.TempDir(), alive: true})
	c := registry.Caller{Project: GlobalProject, Agent: "balin", Role: "reviewer"}

	var out bytes.Buffer
	code, err := e.CmdTasks(c, []string{"sd-39dad3"}, &out)
	if err != nil {
		t.Fatalf("an unknown id came back as a hub fault, which escalates the agent: %v", err)
	}
	if code == 0 {
		t.Error("an unknown id is still a refusal, so the code must be non-zero")
	}
	// Holding no review, its only reachable backlog is the shared one — saying so beats "not found",
	// which reads as the task having been deleted.
	if !strings.Contains(out.String(), "pooled reviewer") {
		t.Errorf("the answer should explain why the id is out of reach, got %q", out.String())
	}
}

// TestLintOnAnUnreachablePRIsAnsweredNotEscalated: `sindri lint <pr-id>` for a PR the caller cannot
// reach is an ANSWER. Returned as an error it reaches AgentExec as a hub failure, which
// auto-escalates — balin was stopped for naming pr-sd-19130a, a PR in another project. approve,
// reject and show all learned this; lint was the one verb left behind.
func TestLintOnAnUnreachablePRIsAnsweredNotEscalated(t *testing.T) {
	st, _, c := foreignPRFixture(t)
	e := New(st, &stubDeps{root: t.TempDir(), alive: true})

	var out bytes.Buffer
	code, err := e.CmdLint(c, []string{"pr-9"}, &out)
	if err != nil {
		t.Fatalf("an unreachable PR came back as a hub fault, which escalates the caller: %v", err)
	}
	if code == 0 {
		t.Error("an unreachable PR is still a refusal, so the code must be non-zero")
	}
	if !strings.Contains(out.String(), "No PR pr-9") {
		t.Errorf("the caller must be told the id is out of its reach, got %q", out.String())
	}
}
