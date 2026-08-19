package api

import "testing"

// TestNeedsUserIsEveryStateOnlyAHumanClears walks the states an agent can wear. The ones that count
// share one property and not a family resemblance: nothing the agent or the hub does resolves them.
func TestNeedsUserIsEveryStateOnlyAHumanClears(t *testing.T) {
	for _, s := range []string{StatusBlocked, StatusSignedOut, StatusFull, StatusStalled, StatusEscalated, StatusLaunchFailed} {
		if !AgentNeedsUser(AgentView{Status: s}) {
			t.Errorf("AgentNeedsUser(%q) = false — that agent holds its work and nobody but a human can move it", s)
		}
	}
	for _, s := range []string{"idle", "working", "submitted", "planning", "collab", "reviewing", "down", StatusUnknown, ""} {
		if AgentNeedsUser(AgentView{Status: s}) {
			t.Errorf("AgentNeedsUser(%q) = true — marking this makes the glyph mean 'an agent exists'", s)
		}
	}
}

// TestIdleIsNeverTheUsersProblem is the line the marker and the idle observer share. An agent
// waiting for work is functioning normally; one idle beside work it could claim is a dispatch fault
// the hub nudges. Neither asks anything of the user, so neither may be counted.
func TestIdleIsNeverTheUsersProblem(t *testing.T) {
	if AgentNeedsUser(AgentView{Status: "idle"}) {
		t.Error("an idle agent is waiting for work, not for the user")
	}
}

// TestRetiredNeverCounts pins the states retirement actually reaches. A retired agent keeps
// running: it fills its context and reads "full", or holds work, stands still and reads "stalled".
// Both pass the switch, so retirement has to be tested against THOSE — against "idle" the case
// passes whatever Retired says, and proves nothing. Retiring a full worker is the ordinary way to
// wind one down, and a marker that stuck to it would sit on the handle until it was deleted.
func TestRetiredNeverCounts(t *testing.T) {
	for _, s := range []string{StatusFull, StatusStalled, StatusBlocked, StatusSignedOut} {
		if !AgentNeedsUser(AgentView{Status: s}) {
			t.Fatalf("%q must count while running, or this test proves nothing about retirement", s)
		}
		if AgentNeedsUser(AgentView{Status: s, Retired: true}) {
			t.Errorf("a retired agent reading %q was wound down deliberately; the decision is made", s)
		}
	}
}

// TestARetiredAgentsQuestionStillCounts is the exception to the rule above, and it turns on WHO put
// the agent in the state. Retirement is excluded because it reaches full and stalled by itself, so a
// marker there would never clear — but nothing about winding an agent down asks a question in its
// name, and a retired agent still finishes what it holds. Its question is unanswered either way.
func TestARetiredAgentsQuestionStillCounts(t *testing.T) {
	if !AgentNeedsUser(AgentView{Status: StatusEscalated, Retired: true}) {
		t.Error("a retired agent that asked the user something is still waiting on the answer")
	}
}

// TestCountAgentsNeedingUserCountsAgentsNotReasons: an agent both full and stalled wears one word,
// and either way it is one thing to attend to.
func TestCountAgentsNeedingUserCountsAgentsNotReasons(t *testing.T) {
	got := CountAgentsNeedingUser([]AgentView{
		{Name: "a", Status: StatusStalled, ContextTokens: 190_000, ContextWindow: 200_000},
		{Name: "b", Status: "working"},
		{Name: "c", Status: StatusBlocked},
	})
	if got != 2 {
		t.Errorf("CountAgentsNeedingUser = %d, want 2", got)
	}
}

// TestPRNeedsUserCoversEveryWaitingState: an approved PR waits on the merge, which no agent may
// perform; an open one with no reviewer alive waits on a review that is never coming; an interim
// one is user-gated by design; a merge-failed one is stuck hardest of all, since no verb accepts it
// and nothing retries it. All four look like ordinary rows, which is why they are counted rather
// than left to be noticed.
func TestPRNeedsUserCoversEveryWaitingState(t *testing.T) {
	live := []AgentView{{Project: "p", Name: "dvalin", Role: "reviewer", Status: "idle"}}
	none := []AgentView{{Project: "p", Name: "dvalin", Role: "worker", Status: "working"}}
	cases := []struct {
		name   string
		pr     PR
		agents []AgentView
		want   bool
	}{
		{"approved waits on the merge", PR{Project: "p", Status: "approved"}, live, true},
		{"approved waits even with nobody about", PR{Project: "p", Status: "approved"}, none, true},
		{"open with no reviewer is stranded", PR{Project: "p", Status: "open"}, none, true},
		{"open with a reviewer running is not", PR{Project: "p", Status: "open"}, live, false},
		{"open, unassigned, reviewer running: it picks one up", PR{Project: "p", Status: "open", Reviewer: ""}, live, false},
		{"an interim PR is the user's from the start", PR{Project: "p", Status: "open", Kind: "interim"}, live, true},
		{"a milestone blocks its agent until you merge", PR{Project: "p", Status: "open", Kind: "interim"}, none, true},
		{"a rejected interim waits on its author", PR{Project: "p", Status: "rejected", Kind: "interim"}, live, false},
		{"rejected waits on its author", PR{Project: "p", Status: "rejected"}, none, false},
		{"merged waits on nobody", PR{Project: "p", Status: "merged"}, none, false},
		{"scrapped waits on nobody", PR{Project: "p", Status: "scrapped"}, none, false},
		// merge-failed: a restart caught the merge in flight, so nobody knows whether base carries
		// it. No agent path touches it and no verb accepts it, so it moves only when a person looks.
		{"merge-failed is the most stuck of all", PR{Project: "p", Status: "merge-failed"}, live, true},
		{"a merge under way waits on the merge, not on you", PR{Project: "p", Status: "merging"}, live, false},
	}
	for _, c := range cases {
		if got := PRNeedsUser(c.pr, c.agents); got != c.want {
			t.Errorf("%s: PRNeedsUser = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestALiveReviewerIsOneWhosePodIsUp: the words that mean "not running" are AgentNotUp's list, so a
// reviewer that has never been started strands the queue and one merely idle does not.
func TestALiveReviewerIsOneWhosePodIsUp(t *testing.T) {
	for _, s := range []string{"idle", "working", "reviewing"} {
		if !AnyLiveReviewer([]AgentView{{Project: "a", Role: "reviewer", Status: s}}, "a") {
			t.Errorf("a reviewer reading %q is running and will pick up reviews", s)
		}
	}
	for _, s := range []string{"down", StatusUnknown, "launching", "stopping", ""} {
		if AnyLiveReviewer([]AgentView{{Project: "a", Role: "reviewer", Status: s}}, "a") {
			t.Errorf("a reviewer reading %q has no pod to review with", s)
		}
	}
	roster := []AgentView{{Project: "a", Role: "worker", Status: "working"}, {Project: "a", Role: "planner", Status: "idle"}}
	if AnyLiveReviewer(roster, "a") {
		t.Error("no reviewer on the roster means no review is coming, whatever else is running")
	}
}

// TestARetiredReviewerIsNotLive: retirement hands off every automatic behaviour, including being
// counted as the reviewer who will pick this up — its status still reads idle (the pod is up), but
// it will never be assigned the review, so a PR waiting on it must escalate rather than sit believing
// someone has it.
func TestARetiredReviewerIsNotLive(t *testing.T) {
	roster := []AgentView{{Project: "a", Role: "reviewer", Status: "idle", Retired: true}}
	if AnyLiveReviewer(roster, "a") {
		t.Error("a retired reviewer must not count as live — it will never take the review")
	}
}

// TestLivenessIsPerRepo is the case a single-project table cannot see. Review assignment reads ONE
// project roster (-> workflow.freeReviewer), so a reviewer running in repo A is never handed repo
// B's PR: B's queue is stranded exactly as if no reviewer existed, and that is what the marker is
// for. Fleet-wide liveness answered "someone is up" and left it unmarked for ever.
func TestLivenessIsPerRepo(t *testing.T) {
	fleet := []AgentView{{Project: "a", Name: "fili", Role: "reviewer", Status: "idle"}}
	if !AnyLiveReviewer(fleet, "a") {
		t.Error("repo a has a reviewer running")
	}
	if AnyLiveReviewer(fleet, "b") {
		t.Error("repo b has none — a reviewer in another repo will never be handed its PRs")
	}
	if !PRNeedsUser(PR{Project: "b", ID: "pr-1", Status: "open"}, fleet) {
		t.Error("an open PR in a repo with no reviewer waits on the user, whatever runs elsewhere")
	}
	if PRNeedsUser(PR{Project: "a", ID: "pr-2", Status: "open"}, fleet) {
		t.Error("the same PR in the repo that HAS the reviewer is not waiting on anybody")
	}
}

// TestCountPRsNeedingUserCountsPRsNotReasons: the count is of things to attend to, and one PR is
// one of those however many ways it qualifies.
func TestCountPRsNeedingUserCountsPRsNotReasons(t *testing.T) {
	prs := []PR{
		{Project: "p", ID: "a", Status: "approved"}, {Project: "p", ID: "b", Status: "open"},
		{Project: "p", ID: "c", Status: "merged"}, {Project: "p", ID: "d", Status: "open", Kind: "interim"},
		{Project: "p", ID: "e", Status: "merge-failed"},
	}
	if got := CountPRsNeedingUser(prs, nil); got != 4 {
		t.Errorf("with no reviewer alive: %d, want 4 (approved, open, interim, merge-failed)", got)
	}
	live := []AgentView{{Project: "p", Role: "reviewer", Status: "idle"}}
	if got := CountPRsNeedingUser(prs, live); got != 3 {
		t.Errorf("with a reviewer running: %d, want 3 (approved, interim, merge-failed)", got)
	}
}

// TestEveryReasonIsInPRWaits: callers group over PRWaits (the CLI's summary does), so a reason the
// rule can return but the list omits would be counted and then silently dropped from the output.
func TestEveryReasonIsInPRWaits(t *testing.T) {
	live := []AgentView{{Project: "p", Role: "reviewer", Status: "idle"}}
	reached := map[PRWait]bool{
		PRWaitReason(PR{Project: "p", Status: "merge-failed"}, live):          true,
		PRWaitReason(PR{Project: "p", Status: "approved"}, live):              true,
		PRWaitReason(PR{Project: "p", Status: "open", Kind: "interim"}, live): true,
		PRWaitReason(PR{Project: "other", Status: "open"}, live):              true,
	}
	if len(reached) != len(PRWaits) {
		t.Fatalf("reached %d distinct reasons, PRWaits lists %d", len(reached), len(PRWaits))
	}
	for _, w := range PRWaits {
		if !reached[w] {
			t.Errorf("PRWaits lists %q, but no PR in this test produces it — one of the two is wrong", w)
		}
		if w == PRWaitNone {
			t.Error("PRWaitNone is the absence of a reason; listing it would have callers group on it")
		}
	}
}
