package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// escalatedWorker seeds a worker holding a task and escalates it through the verb, returning the
// hub and its project store. Through the verb rather than the store, so what the tests assert on is
// the state the agent can actually put itself into.
func escalatedWorker(t *testing.T, question string) (*Hub, *store.ProjectStore) {
	t.Helper()
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-1", Title: "a task", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-1", Branch: "td-1", Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	if out, code := execAs(t, h, "dvalin", append([]string{"escalate"}, strings.Fields(question)...)...); code != 0 {
		t.Fatalf("escalate failed (%d): %s", code, out)
	}
	return h, ps
}

// landsWork is the classification the hold turns on (-> heldByEscalation): the verbs that LAND work,
// as against the reads, the records and the proposals. The rule is stated for every role, so it is
// checked for every role — a list that only covered the worker's verbs is how a planner's `openspec
// submit` stayed open while the hub told it everything was shut.
var landsWork = map[string]bool{
	"next": true, "submit": true, "contribute": true, "checkpoint": true,
	"approve": true, "reject": true, "openspec": true,
}

// escalatedAs seeds an agent of the given role, escalates it through the verb, and returns the hub.
func escalatedAs(t *testing.T, role, question string) *Hub {
	t.Helper()
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: role, Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	if out, code := execAs(t, h, "dvalin", append([]string{"escalate"}, strings.Fields(question)...)...); code != 0 {
		t.Fatalf("escalate as %s failed (%d): %s", role, code, out)
	}
	return h
}

// TestEveryRolesLandingVerbsAreHeld walks each role's WHOLE advertised surface and checks the
// partition: every verb that lands work refuses with the agent's own question, and nothing else
// refuses because of the escalation. This is the rule the spec states — "every verb that lands work",
// for every role — rather than the enumeration one role's verbs happen to make.
func TestEveryRolesLandingVerbsAreHeld(t *testing.T) {
	const q = "which of the two schemas is authoritative?"
	for _, role := range []string{"worker", "reviewer", "planner", "coauthor"} {
		h := escalatedAs(t, role, q)
		cmds, err := h.AgentCommands(testProject, "dvalin")
		if err != nil {
			t.Fatalf("%s: agent commands: %v", role, err)
		}
		if len(cmds) < 5 {
			t.Fatalf("%s: only %d verbs on the surface — the roster is not being read", role, len(cmds))
		}
		held := 0
		for _, c := range cmds {
			byEscalation := strings.Contains(c.Unavailable, q)
			switch {
			case landsWork[c.Name] && !byEscalation:
				t.Errorf("%s: %s lands work but is not held while escalated (reason %q)", role, c.Name, c.Unavailable)
			case !landsWork[c.Name] && byEscalation:
				t.Errorf("%s: %s neither lands work nor should be held by the escalation: %q", role, c.Name, c.Unavailable)
			case byEscalation:
				held++
				if !strings.Contains(c.Unavailable, "sindri resume") {
					t.Errorf("%s: %s's refusal should name the verb that clears it: %q", role, c.Name, c.Unavailable)
				}
			}
		}
		// A role with nothing to land is a role this rule says nothing about — the coauthor, which
		// commits with git itself. Every other role must have had something shut, or the loop above
		// passed by finding no landing verbs at all.
		if held == 0 && role != "coauthor" {
			t.Errorf("%s: no verb was held — the hold is not reaching this role", role)
		}
	}
}

// TestEscalatingHoldsTheWorkVerbsAndKeepsTheReads is the shape of the whole feature: an agent stopped
// on the user's decision may still read — it has to re-read the task and its diff to act on an answer
// — but nothing it does may advance the work, and each refusal quotes its own question back so the
// hold reads as its own doing rather than a hub that has gone silent.
func TestEscalatingHoldsTheWorkVerbsAndKeepsTheReads(t *testing.T) {
	const q = "drop the two callers or keep both?"
	h, _ := escalatedWorker(t, q)

	for _, verb := range []string{"submit", "contribute", "next", "checkpoint"} {
		reason := blockedFor(t, h, "dvalin", verb)
		if reason == "" {
			t.Errorf("%s must be held back while escalated", verb)
			continue
		}
		if !strings.Contains(reason, q) {
			t.Errorf("%s's refusal should quote the question: %q", verb, reason)
		}
		if !strings.Contains(reason, "sindri resume") {
			t.Errorf("%s's refusal should name the verb that clears it: %q", verb, reason)
		}
	}
	// Reads stay: an answer it cannot act on is no answer. `git` is the whole read/restore subset an
	// agent has instead of git, so losing it would leave it unable to look at its own change.
	for _, verb := range []string{"status", "task", "show", "prs", "log", "git"} {
		if reason := blockedFor(t, h, "dvalin", verb); reason != "" {
			t.Errorf("%s must stay open while escalated: %q", verb, reason)
		}
	}
	// And the way out is never shut.
	if reason := blockedFor(t, h, "dvalin", "resume"); reason != "" {
		t.Errorf("resume must be open to an escalated agent: %q", reason)
	}
}

// blockedFor returns why an agent cannot run verb right now, "" when it can. It reads the advertised
// surface, since a verb held back must still be LISTED with its reason — an agent shown a list
// without submit concluded the hub was broken.
func blockedFor(t *testing.T, h *Hub, agent, verb string) string {
	t.Helper()
	cmds, err := h.AgentCommands(testProject, agent)
	if err != nil {
		t.Fatalf("agent commands: %v", err)
	}
	for _, c := range cmds {
		if c.Name == verb {
			return c.Unavailable
		}
	}
	t.Fatalf("%q is not on %s's surface at all — a verb it may be pointed at must be listed", verb, agent)
	return ""
}

// TestAnEscalatedReviewerGivesNoVerdict: a verdict is landing work on somebody else's behalf, and a
// reviewer that has stopped on a question about the very thing it is judging must not deliver one.
// Its own two verbs never appear on a worker's surface, so the worker case above cannot reach them.
func TestAnEscalatedReviewerGivesNoVerdict(t *testing.T) {
	const q = "the diff contradicts the spec — is the spec the one that is wrong?"
	h := escalatedAs(t, "reviewer", q)
	for _, verb := range []string{"approve", "reject"} {
		reason := blockedFor(t, h, "dvalin", verb)
		if !strings.Contains(reason, q) {
			t.Errorf("%s should be held with the reviewer's own question: %q", verb, reason)
		}
	}
	// It can still read the PR it is judging, which is how it acts on the answer.
	for _, verb := range []string{"show", "prs", "task"} {
		if reason := blockedFor(t, h, "dvalin", verb); reason != "" {
			t.Errorf("%s must stay open to an escalated reviewer: %q", verb, reason)
		}
	}
}

// TestAnEscalatedPlannerShipsNothing is the bug the review caught: `openspec submit` is a planner's
// submit under another name, and it carried no gate — so the hub told a planner everything that lands
// work was shut while its one landing verb was open. Proposing stays open: the user rules on a
// proposal before it becomes work, so a planner tidying the backlog while it waits harms nothing.
func TestAnEscalatedPlannerShipsNothing(t *testing.T) {
	const q = "one change or three?"
	h := escalatedAs(t, "planner", q)
	if reason := blockedFor(t, h, "dvalin", "openspec"); !strings.Contains(reason, q) {
		t.Errorf("openspec should be held with the planner's own question: %q", reason)
	}
	for _, verb := range []string{"create-task", "edit-task", "prioritise-task", "task", "state"} {
		if reason := blockedFor(t, h, "dvalin", verb); reason != "" {
			t.Errorf("%s only proposes or records, so it stays open while escalated: %q", verb, reason)
		}
	}
}

// TestAHeldVerbAlsoREFUSESToRun closes the gap between "advertised as unavailable" and "will not
// run". The surface and the dispatcher read the same predicate, but only this proves it: the verb is
// executed for real, and what it would have changed is checked to be unchanged.
func TestAHeldVerbAlsoREFUSESToRun(t *testing.T) {
	const q = "drop the two callers or keep both?"
	h, ps := escalatedWorker(t, q)
	out, code := execAs(t, h, "dvalin", "submit", "done with it")
	if code == 0 {
		t.Errorf("submit ran while escalated: %s", out)
	}
	if !strings.Contains(out, q) {
		t.Errorf("the refusal should quote the question: %s", out)
	}
	prs, err := ps.PRs()
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 0 {
		t.Errorf("a held submit registered a PR anyway: %v", prs)
	}
	st, _ := ps.GetState("dvalin")
	if st.Phase != "working" || st.Escalation != q {
		t.Errorf("nothing about the agent should have moved, got phase %q escalation %q", st.Phase, st.Escalation)
	}
}

// TestAnEscalationWithoutAQuestionIsRefused: an escalation that states no question tells the user
// only that something is wrong, which is the part they can already see from the board.
func TestAnEscalationWithoutAQuestionIsRefused(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	out, code := execAs(t, h, "dvalin", "escalate")
	if code == 0 {
		t.Errorf("a questionless escalation must be refused: %s", out)
	}
	if !strings.Contains(out, "REQUIRED") {
		t.Errorf("the refusal should say the question is required: %s", out)
	}
	st, err := ps.GetState("dvalin")
	if err != nil {
		t.Fatal(err)
	}
	if st.Escalation != "" {
		t.Errorf("nothing should have been recorded: %q", st.Escalation)
	}
}

// TestTheQuestionOutlivesTheSession: the escalation's value is that a human, or the next agent, can
// read what was asked long after the terminal that asked it is gone — so it goes in the activity log
// AND on the task, which is what somebody opening the work actually reads.
func TestTheQuestionOutlivesTheSession(t *testing.T) {
	const q = "keep both callers?"
	_, ps := escalatedWorker(t, q)

	evs, err := ps.Events("dvalin", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range evs {
		if e.Type == "escalate" && e.Payload == q {
			found = true
		}
	}
	if !found {
		t.Errorf("the question should be in the activity log, got %v", evs)
	}
	cs, err := ps.Comments("td-1")
	if err != nil || len(cs) != 1 {
		t.Fatalf("comments on td-1 = %v (err %v), want the escalation", cs, err)
	}
	if !strings.Contains(cs[0].Body, q) || cs[0].Author != "dvalin" {
		t.Errorf("the comment should carry the question, attributed to the agent: %+v", cs[0])
	}
}

// TestResumingIsRecordedAndReopensTheWork: the agent clears its own escalation, because only it knows
// it has understood the answer.
func TestResumingIsRecordedAndReopensTheWork(t *testing.T) {
	h, ps := escalatedWorker(t, "keep both callers?")
	if out, code := execAs(t, h, "dvalin", "resume"); code != 0 {
		t.Fatalf("resume failed (%d): %s", code, out)
	}
	st, err := ps.GetState("dvalin")
	if err != nil {
		t.Fatal(err)
	}
	if st.Escalation != "" {
		t.Errorf("resume should clear the escalation, got %q", st.Escalation)
	}
	if reason := blockedFor(t, h, "dvalin", "submit"); reason != "" {
		t.Errorf("submit should be open again after resuming: %q", reason)
	}
	// Once cleared there is nothing to clear, and the surface says so rather than offering a no-op.
	if reason := blockedFor(t, h, "dvalin", "resume"); reason == "" {
		t.Error("resume should be held back when nothing is escalated")
	}
	evs, _ := ps.Events("dvalin", 10)
	last := evs[len(evs)-1]
	if last.Type != "resume" {
		t.Errorf("the release should be recorded too, got %+v", last)
	}
}

// TestTheUserCanClearAnEscalationToo: the agent's own resume is the normal path, but an agent may be
// deleted, restarted, or simply wrong that it was blocked — and an escalation nobody can clear is a
// stuck agent by another name. This is the host route (POST /agent/resume, `sindri agent resume`).
func TestTheUserCanClearAnEscalationToo(t *testing.T) {
	h, ps := escalatedWorker(t, "keep both callers?")
	if err := h.Resume(testProject, "dvalin", "escalation cleared by the user"); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	st, _ := ps.GetState("dvalin")
	if st.Escalation != "" {
		t.Errorf("the user's clear should release the agent, got %q", st.Escalation)
	}
	// Idempotent, so neither caller has to ask whether there was one first.
	if err := h.Resume(testProject, "dvalin", "again"); err != nil {
		t.Errorf("clearing nothing should be a no-op, got %v", err)
	}
}

// TestReEscalatingReplacesTheQuestion: escalate stays open while escalated, because a badly-put
// question that cannot be re-put leaves the agent with nothing to do but wait for an answer to the
// wrong thing.
func TestReEscalatingReplacesTheQuestion(t *testing.T) {
	h, ps := escalatedWorker(t, "which schema?")
	if out, code := execAs(t, h, "dvalin", "escalate", "clearer:", "which", "of", "the", "two", "schemas", "wins?"); code != 0 {
		t.Fatalf("re-escalate failed (%d): %s", code, out)
	}
	st, _ := ps.GetState("dvalin")
	if st.Escalation != "clearer: which of the two schemas wins?" {
		t.Errorf("the second question should stand, got %q", st.Escalation)
	}
}

// TestAnEscalatedAgentWearsTheStatusAndNeedsTheUser ties the durable state to the one word every
// front-end renders, and to the marker that counts it. Without the marker an escalated agent is
// indistinguishable from one with nothing to do, which is the whole failure being fixed.
func TestAnEscalatedAgentWearsTheStatusAndNeedsTheUser(t *testing.T) {
	if got := overlayEscalation("working", "which schema?"); got != api.StatusEscalated {
		t.Errorf("status = %q, want %q", got, api.StatusEscalated)
	}
	// It outranks the words describing a screen: a stalled reading is the same standing still seen
	// without the reason for it, and a runtime block is a different remedy wearing one word.
	for _, was := range []string{api.StatusStalled, api.StatusBlocked, "idle", "submitted"} {
		if got := overlayEscalation(was, "which schema?"); got != api.StatusEscalated {
			t.Errorf("overlayEscalation(%q) = %q, want %q", was, got, api.StatusEscalated)
		}
	}
	// Two still outrank it, both saying the answer cannot be DELIVERED until something else is fixed.
	for _, was := range []string{"down", api.StatusUnknown, "launching", api.StatusSignedOut} {
		if got := overlayEscalation(was, "which schema?"); got != was {
			t.Errorf("overlayEscalation(%q) = %q, want it unchanged", was, got)
		}
	}
	if got := overlayEscalation("working", ""); got != "working" {
		t.Errorf("an agent with no escalation must be untouched, got %q", got)
	}
	if !api.AgentNeedsUser(api.AgentView{Status: api.StatusEscalated}) {
		t.Error("an escalated agent must count as needing the user")
	}
}
