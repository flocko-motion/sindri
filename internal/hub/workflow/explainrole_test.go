package workflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// roleFixture is a project with one open PR per shape a reviewer's pool can be in.
func roleFixture(t *testing.T) (*Engine, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return newEngine(st, &stubDeps{root: t.TempDir()}), st.For("repo")
}

// reasons maps each PR to the standing the explanation gave it.
func reasons(x api.NextExplain) map[string]api.Reviewability {
	out := map[string]api.Reviewability{}
	for _, p := range x.PRs {
		out[p.ID] = p.Why
	}
	return out
}

// TestAReviewerIsAnsweredWithoutOneRunning is the point of the flag: "would a reviewer have
// anything to do if I started one" previously took starting one and watching.
func TestAReviewerIsAnsweredWithoutOneRunning(t *testing.T) {
	e, ps := roleFixture(t)
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "sd-1", Agent: "bombur", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddReview("pr-1", "check it"); err != nil {
		t.Fatal(err)
	}
	x, err := e.ExplainNext("repo", "", "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if x.PickPR == nil || x.PickPR.ID != "pr-1" {
		t.Fatalf("pick = %+v, want the unclaimed review of pr-1", x.PickPR)
	}
	if x.Pick != nil || len(x.Tasks) != 0 {
		t.Error("a reviewer is served PRs, so the task half must stay empty")
	}
}

// TestEveryReasonAPRGoesUnreviewed covers the states that let a PR sit while a reviewer idled —
// the case this command exists to make answerable in one call.
func TestEveryReasonAPRGoesUnreviewed(t *testing.T) {
	e, ps := roleFixture(t)
	for _, pr := range []store.PR{
		{ID: "pr-waiting", Task: "sd-1", Status: "open"},
		{ID: "pr-claimed", Task: "sd-2", Status: "open"},
		{ID: "pr-unasked", Task: "sd-3", Status: "open"},
		{ID: "pr-interim", Task: "sd-4", Status: "open", Kind: "interim"},
		{ID: "pr-approved", Task: "sd-5", Status: "approved"},
		{ID: "pr-rejected", Task: "sd-6", Status: "rejected"},
		{ID: "pr-merging", Task: "sd-7", Status: "merging"},
		{ID: "pr-halfmerged", Task: "sd-8", Status: "merge-failed", Base: "main"},
		{ID: "pr-merged", Task: "sd-9", Status: "merged"},
	} {
		if err := ps.PutPR(pr); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ps.AddReview("pr-waiting", "check it"); err != nil {
		t.Fatal(err)
	}
	claimed, err := ps.AddReview("pr-claimed", "check it")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.AssignReview(claimed, "dvalin"); err != nil {
		t.Fatal(err)
	}

	x, err := e.ExplainNext("repo", "", "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	got := reasons(x)
	for id, want := range map[string]api.Reviewability{
		"pr-waiting":  api.ReviewWaiting,
		"pr-claimed":  api.ReviewInHand,
		"pr-unasked":  api.ReviewUnrequested,
		"pr-interim":  api.ReviewInterim,
		"pr-approved": api.ReviewApproved,
		// Rejected is the commonest of these and the one a single "settled" got wrong: the author
		// is revising, a resubmit reopens the PR, and nobody is waiting on a reviewer meanwhile.
		"pr-rejected":   api.ReviewRejected,
		"pr-merging":    api.ReviewMerging,
		"pr-halfmerged": api.ReviewMergeFailed,
	} {
		if got[id] != want {
			t.Errorf("%s: %q, want %q", id, got[id], want)
		}
	}
	if _, listed := got["pr-merged"]; listed {
		t.Error("a merged PR is off the board, not an unanswered question")
	}
	if x.PickPR == nil || x.PickPR.ID != "pr-waiting" {
		t.Errorf("pick = %+v, want the one unclaimed review", x.PickPR)
	}
	// The one that says whose move it is: a claimed review names its reviewer, since "already being
	// reviewed" without a name leaves the user hunting for who.
	for _, p := range x.PRs {
		if p.ID == "pr-claimed" && !strings.Contains(p.Note, "dvalin") {
			t.Errorf("pr-claimed note = %q, want the reviewer named", p.Note)
		}
	}
}

// TestARolesServedFromNoPoolSaysSo: an empty list reads as "no work", when the truth is that this
// is not how the role gets any.
func TestARolesServedFromNoPoolSaysSo(t *testing.T) {
	e, _ := roleFixture(t)
	for _, role := range []string{"planner", "coauthor"} {
		x, err := e.ExplainNext("repo", "", role)
		if err != nil {
			t.Fatal(err)
		}
		if x.RoleNote == "" {
			t.Errorf("%s: no note — an empty list would read as an empty backlog", role)
		}
		if len(x.Tasks) != 0 || len(x.PRs) != 0 {
			t.Errorf("%s: served from no pool, so neither list should be filled", role)
		}
	}
}

// TestAnAgentAndARoleCannotBeAskedTogether: an agent HAS a role, so the two can contradict, and
// picking a winner would hide the contradiction instead of showing it.
func TestAnAgentAndARoleCannotBeAskedTogether(t *testing.T) {
	e, ps := roleFixture(t)
	if err := ps.PutAgent(store.Agent{Name: "bombur", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ExplainNext("repo", "bombur", "reviewer"); err == nil {
		t.Error("asking about an agent and a role at once must be refused")
	}
}

// TestAnAgentIsAnsweredForItsOwnRole: a reviewer asked about by name is answered from the pool it
// is actually served from, rather than being told about a backlog it will never be offered.
func TestAnAgentIsAnsweredForItsOwnRole(t *testing.T) {
	e, ps := roleFixture(t)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "sd-1", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	x, err := e.ExplainNext("repo", "dvalin", "")
	if err != nil {
		t.Fatal(err)
	}
	if x.Role != "reviewer" {
		t.Errorf("role = %q, want the agent's own", x.Role)
	}
	if len(x.Tasks) != 0 {
		t.Error("a reviewer is never offered backlog tasks, so listing them answers the wrong question")
	}
}

// TestAReviewerHoldingOneTakesNothing: its own state rules out the pool, exactly as a worker
// holding a task does — the half of the answer that is about the agent rather than the queue.
func TestAReviewerHoldingOneTakesNothing(t *testing.T) {
	e, ps := roleFixture(t)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "sd-1", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	held, err := ps.AddReview("pr-1", "check it")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.AssignReview(held, "dvalin"); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-2", Task: "sd-2", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddReview("pr-2", "check it"); err != nil {
		t.Fatal(err)
	}

	x, err := e.ExplainNext("repo", "dvalin", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(x.AgentNote, "pr-1") {
		t.Errorf("AgentNote = %q, want it to name the review in hand", x.AgentNote)
	}
	if x.PickPR != nil {
		t.Errorf("it holds one already, so nothing is picked for it: %+v", x.PickPR)
	}
}

// TestAnUnknownRoleIsRefused, rather than quietly answered as a worker — a typo would otherwise
// read as a confident answer about the wrong pool.
func TestAnUnknownRoleIsRefused(t *testing.T) {
	e, _ := roleFixture(t)
	if _, err := e.ExplainNext("repo", "", "reviwer"); err == nil {
		t.Error("an unknown role must be refused")
	}
}

// TestANoteNamesACommandThatWorks: a note is advice, and advice that errors is worse than silence —
// especially in the states a user reaches while already confused. Asserted against the GATES
// themselves (Merge takes only an approved PR; approve takes only api.PRApprovable), so it holds for
// whatever wording the notes land on, which is what checking for the word "approve" did not.
func TestANoteNamesACommandThatWorks(t *testing.T) {
	e, ps := roleFixture(t)
	prs := []store.PR{
		{ID: "pr-open", Status: "open"},
		{ID: "pr-approved", Status: "approved"},
		{ID: "pr-rejected", Status: "rejected"},
		{ID: "pr-merging", Status: "merging"},
		{ID: "pr-halfmerged", Status: "merge-failed", Base: "main"},
	}
	byID := map[string]store.PR{}
	for _, pr := range prs {
		if err := ps.PutPR(pr); err != nil {
			t.Fatal(err)
		}
		byID[pr.ID] = pr
	}
	x, err := e.ExplainNext("repo", "", "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if len(x.PRs) != len(prs) {
		t.Fatalf("every PR should be accounted for, got %d of %d", len(x.PRs), len(prs))
	}
	for _, p := range x.PRs {
		pr := byID[p.ID]
		if strings.Contains(p.Note, "pr merge "+p.ID) && pr.Status != "approved" {
			t.Errorf("%s is %s and its note offers `pr merge`, which Merge refuses: %q",
				p.ID, pr.Status, p.Note)
		}
		if strings.Contains(p.Note, "pr approve "+p.ID) && !api.PRApprovable(pr) {
			t.Errorf("%s is %s and its note offers `pr approve`, which the approve gate refuses: %q",
				p.ID, pr.Status, p.Note)
		}
	}
	notes := map[string]string{}
	for _, p := range x.PRs {
		notes[p.ID] = p.Note
	}
	if !strings.Contains(notes["pr-approved"], "pr merge pr-approved") {
		t.Errorf("an approved PR wants merging, and the note should say so: %q", notes["pr-approved"])
	}
	// The half-merged one still has to be USEFUL: no verb reaches out of that status, so it says
	// what is unknown and where to look rather than nothing at all.
	if !strings.Contains(notes["pr-halfmerged"], "main") {
		t.Errorf("a half-merged PR must point at the base branch to inspect: %q", notes["pr-halfmerged"])
	}
}

// TestAStaleHoldIsNotAHold: reviewDirective closes a review whose PR has left "open" and claims the
// next one, so reporting the reviewer as busy describes a state its very next ask undoes. Reachable
// in ordinary use — a human `pr approve` leaves the reviewer's own row unverdicted.
func TestAStaleHoldIsNotAHold(t *testing.T) {
	e, ps := roleFixture(t)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "sd-1", Status: "approved"}); err != nil {
		t.Fatal(err)
	}
	stale, err := ps.AddReview("pr-1", "check it")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.AssignReview(stale, "dvalin"); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-2", Task: "sd-2", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddReview("pr-2", "check it"); err != nil {
		t.Fatal(err)
	}

	x, err := e.ExplainNext("repo", "dvalin", "")
	if err != nil {
		t.Fatal(err)
	}
	if x.AgentNote != "" {
		t.Errorf("AgentNote = %q — the held PR is settled, so the hold is already released", x.AgentNote)
	}
	if x.PickPR == nil || x.PickPR.ID != "pr-2" {
		t.Errorf("pick = %+v, want pr-2 — what its next ask would actually claim", x.PickPR)
	}
}
