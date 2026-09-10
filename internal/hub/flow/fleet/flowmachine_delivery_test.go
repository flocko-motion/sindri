// package: hub/flow/fleet / flowmachine_delivery_test
// type:    logic (which messages must be READ, and which only wake)
// job:     walk two real senders end to end — a rejection carrying feedback, and a stall nudge —
// and pin that one is mailed and pushed while the other only wakes.
// limits:  these two paths. That EVERY sender states both properties is a guard over the acting
// tree (-> internal/arch).
package fleet

import (
	agent "github.com/flo-at/sindri/internal/hub/flow/agent"
	"path/filepath"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestARejectionIsMailedAndTheNudgeIsNot walks two real senders end to end, which is what the rule
// actually claims. A rejection with its feedback must not be lost to an agent that was away, so it is
// mailed AND pushed. A stall nudge is the opposite case: waking is its entire purpose, it fires again
// next tick, and mailing it would keep chatter for ever — which is the argument that makes unbounded
// retention affordable in the first place.
func TestARejectionIsMailedAndTheNudgeIsNot(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "bombur", Role: "worker", Workspace: ".worktrees/bombur"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "td-1", Agent: "bombur", Branch: "td-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "td-1", Branch: "td-1", Phase: "submitted"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{Root: t.TempDir(), Alive: true}
	e := newEngine(t, st, deps)

	if err := e.prAct().RejectPR("proj", "pr-1", "needs another pass"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if len(deps.Delivered) != 1 || !deps.Delivered[0].Mail || !deps.Delivered[0].Push {
		t.Fatalf("a rejection must be mailed and pushed, got %+v", deps.Delivered)
	}
	// And it says who rejected it, which is the half sd-bc3a1f added: an agent weights a message by
	// its sender, and "the hub" would be a worse answer than the truth.
	if deps.Delivered[0].Sender == "" {
		t.Error("a rejection should name its author as the sender")
	}

	if !e.roleAct().NudgeStalled("proj", "bombur", flowtest.Saying("idle"), agent.StallDwell+time.Minute) {
		t.Fatal("a rejected worker gone quiet past the dwell should be nudged")
	}
	if len(deps.Delivered) != 2 || deps.Delivered[1].Mail || !deps.Delivered[1].Push {
		t.Fatalf("a stall nudge must be push-only, got %+v", deps.Delivered)
	}
}
