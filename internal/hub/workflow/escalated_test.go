package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestAnEscalatedAgentIsToldWhatItAsked is the rehydration case: a relaunched agent remembers nothing
// of escalating, so the directive — the one thing it runs on waking — has to say both that it is
// escalated and what the question was, or it tries to carry on and is refused by every work verb
// without knowing why.
func TestAnEscalatedAgentIsToldWhatItAsked(t *testing.T) {
	e, ps := quietWorkerHoldingWork(t, &stubDeps{})
	const q = "the task says migrate the column, but two callers read it — drop them or keep both?"
	if err := ps.SetEscalation("dvalin", q); err != nil {
		t.Fatal(err)
	}
	dir, err := e.AgentDirective(context.Background(), "proj", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, q) {
		t.Errorf("the directive must repeat the question back: %q", dir)
	}
	for _, want := range []string{"sindri resume", "ESCALATED"} {
		if !strings.Contains(dir, want) {
			t.Errorf("the directive should name %q: %q", want, dir)
		}
	}
	// It holds a task in "working", so without the escalation this would be the work directive — the
	// point is that the escalation outranks it rather than being appended to it.
	if strings.Contains(dir, "Work on task") {
		t.Errorf("an escalated agent must not be told to get on with the task: %q", dir)
	}
}

// TestEscalationWordingDoesNotAssumeTheAgentAskedOrInviteSelfResume is sd-350519's rejection round:
// a hub-raised escalation is not the agent stopping to ask a question, so DirEscalated/ReplyEscalated
// must not open by claiming it did, and the closing "resume anyway if you now see the answer" reads
// as permission to route around a hub fault the moment the failing verb happens to work again —
// exactly what auto-escalation exists to remove.
func TestEscalationWordingDoesNotAssumeTheAgentAskedOrInviteSelfResume(t *testing.T) {
	const q = "`sindri git` failed inside the hub, so I stopped."
	dir := DirEscalated(q)
	if strings.Contains(dir, "you stopped and asked") {
		t.Errorf("the directive must not claim the agent asked, false for a hub-raised one: %q", dir)
	}
	if strings.Contains(dir, "resume anyway") || strings.Contains(dir, "no longer needs") {
		t.Errorf("the directive must not invite a self-resume: %q", dir)
	}
	if !strings.Contains(dir, q) {
		t.Errorf("the directive should still repeat the question back: %q", dir)
	}

	reply := ReplyEscalated("submit", q)
	if strings.Contains(reply, "You escalated") {
		t.Errorf("the reply must not claim the agent performed the escalating: %q", reply)
	}
}

// TestAnEscalatedAgentIsNotNudged: it is idle BY INSTRUCTION — the hub told it to wait quietly for an
// answer only the user can give — so the stall nudge would complain about the state the hub is holding
// it in, which is what happened to a retired agent (sd-521867) before that state was exempted.
func TestAnEscalatedAgentIsNotNudged(t *testing.T) {
	e, ps := quietWorkerHoldingWork(t, &stubDeps{})
	if err := ps.SetEscalation("dvalin", "which of the two schemas is authoritative?"); err != nil {
		t.Fatal(err)
	}
	if e.NudgeStalled("proj", "dvalin", "idle", 6*time.Minute) {
		t.Error("an escalated agent was nudged for waiting as it was told to")
	}
	// A cut-off turn is exempt too, unlike the other parked states: there is nothing for a resumed
	// turn to do while every verb that advances the work is refused.
	if e.NudgeStalled("proj", "dvalin", "api-error", 6*time.Minute) {
		t.Error("an escalated agent was asked to resume a turn it has no work to resume into")
	}
}

// TestClearingTheEscalationRestoresTheWorkDirective: resuming must hand the agent back the task it
// still holds, not leave it in a state of its own — the escalation is a hold over the workflow, not a
// replacement for it.
func TestClearingTheEscalationRestoresTheWorkDirective(t *testing.T) {
	e, ps := quietWorkerHoldingWork(t, &stubDeps{})
	if err := ps.SetEscalation("dvalin", "drop them or keep both?"); err != nil {
		t.Fatal(err)
	}
	if err := ps.ClearEscalation("dvalin"); err != nil {
		t.Fatal(err)
	}
	dir, err := e.AgentDirective(context.Background(), "proj", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "sd-1") {
		t.Errorf("a resumed worker should be back on the task it holds: %q", dir)
	}
}

// TestAnEscalationSurvivesAPhaseChange is the durability the state rests on. Every SetState caller
// builds a fresh AgentState from the columns it cares about, so writing the escalation from that
// struct would clear it on the next phase change — and a state an unrelated write can drop is not
// durable, whatever the schema says.
func TestAnEscalationSurvivesAPhaseChange(t *testing.T) {
	_, ps := quietWorkerHoldingWork(t, &stubDeps{})
	const q = "which of the two schemas is authoritative?"
	if err := ps.SetEscalation("dvalin", q); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "sd-1", Branch: "sd-1", Phase: "submitted"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	st, err := ps.GetState("dvalin")
	if err != nil {
		t.Fatal(err)
	}
	if st.Escalation != q {
		t.Errorf("escalation after a phase write = %q, want it untouched", st.Escalation)
	}
	if st.Phase != "submitted" {
		t.Errorf("phase = %q — the phase write itself must still land", st.Phase)
	}
}
