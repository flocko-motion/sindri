package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// addUnreadMail seeds one unread message for an agent and returns its id, for tests that later mark
// it read to move past the deferred-mail step.
func addUnreadMail(t *testing.T, ps *store.ProjectStore, agent string) int64 {
	t.Helper()
	m, err := ps.AddMail(agent, "hub", "[hub] something happened", false, 0)
	if err != nil {
		t.Fatalf("AddMail: %v", err)
	}
	return m.ID
}

// TestMailOutranksRetirement: a retired agent is told, in as many words, not to ask again — the same
// "sit still" instruction escalation carries, so mail must outrank it for the same reason: it is not
// a pending operation, and an unread message here would never be re-served.
func TestMailOutranksRetirement(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)
	mailID := addUnreadMail(t, ps, "dvalin")
	a, _, _ := ps.GetAgent("dvalin")
	a.Retired = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "unread") {
		t.Errorf("directive = %q, want mail delivered ahead of the retirement notice", dir)
	}

	if err := ps.MarkMailRead(mailID); err != nil {
		t.Fatal(err)
	}
	dir, err = e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "retired") {
		t.Errorf("directive = %q, want the retirement notice now the mailbox is empty", dir)
	}
}

// TestMailOutranksAFullContext: a full worker is told not to ask again either, same as retirement —
// fullness is not a pending operation about to resolve, just another "wait for a human" state.
func TestMailOutranksAFullContext(t *testing.T) {
	deps := &stubDeps{ctxTokens: 900_000, ctxWindow: 1_000_000, ctxOK: true}
	e, ps := idleWorkerWithOpenTask(t, deps)
	mailID := addUnreadMail(t, ps, "dvalin")

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "unread") {
		t.Errorf("directive = %q, want mail delivered ahead of the fullness notice", dir)
	}

	if err := ps.MarkMailRead(mailID); err != nil {
		t.Fatal(err)
	}
	dir, err = e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "context is") {
		t.Errorf("directive = %q, want the fullness notice now the mailbox is empty", dir)
	}
}

// TestMailDefersPastAnArmedClear is the OBSERVED bug: an armed clear is about to wipe the agent's
// context, so mail delivered ahead of it would be marked read and then discarded along with the
// context that held it — worse than never having shown it, since an unread message survives the
// clear and is re-served. The clear must resolve first; mail lands only once it has.
func TestMailDefersPastAnArmedClear(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)
	mailID := addUnreadMail(t, ps, "dvalin")
	a, _, _ := ps.GetAgent("dvalin")
	a.ClearArmed = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if dir != DirClearPending {
		t.Errorf("directive = %q, want the clear to fire ahead of the mail it would otherwise discard", dir)
	}
	if n, _ := ps.UnreadMailCount("dvalin"); n != 1 {
		t.Errorf("unread = %d, the message must survive since it was never shown", n)
	}

	// The clear has landed (a real one flips the flag itself); the fresh context now asks again.
	a.ClearArmed = false
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	dir, err = e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "unread") {
		t.Errorf("directive = %q, want mail delivered into the fresh context before any claim", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "" {
		t.Errorf("state.Task = %q, the claim must wait until the mail is read", st.Task)
	}

	if err := ps.MarkMailRead(mailID); err != nil {
		t.Fatal(err)
	}
	dir, err = e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "td-abc123") {
		t.Errorf("directive = %q, want the task claimed now the mailbox is empty", dir)
	}
}

// TestMailDefersPastCompactionOnAClaim: fill past the threshold compacts inline, in the same pass
// that would otherwise hand the task over. Mail must land in the context that compaction just
// produced, not the one it replaced — so it is checked after the compact, before the claim.
func TestMailDefersPastCompactionOnAClaim(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := idleWorkerWithOpenTask(t, deps)
	mailID := addUnreadMail(t, ps, "dvalin")

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "unread") {
		t.Errorf("directive = %q, want mail delivered into the just-compacted context, not the task", dir)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != "dvalin" {
		t.Errorf("compacted = %v, want exactly one Compact(dvalin) fired ahead of the mail", deps.compacted)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "" {
		t.Errorf("state.Task = %q, the claim must wait until the mail is read", st.Task)
	}

	if err := ps.MarkMailRead(mailID); err != nil {
		t.Fatal(err)
	}
	dir, err = e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "td-abc123") {
		t.Errorf("directive = %q, want the task claimed now the mailbox is empty", dir)
	}
}

// TestMailDefersPastAModelChange: a retiering restart is itself a fresh-context boundary — SetModel
// has already compacted and relaunched the pod by the time DirRetiering is returned. Mail is not
// delivered on this call (there is nothing to read it into yet); it lands on the next one, in the
// pod that comes back up.
func TestMailDefersPastAModelChange(t *testing.T) {
	deps := &stubDeps{tierModels: map[string]string{"mid": "big-model"}, currentModel: "small-model"}
	e, ps := idleWorkerWithOpenTask(t, deps)
	addUnreadMail(t, ps, "dvalin")

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if strings.Contains(dir, "unread") {
		t.Errorf("directive = %q, want the model change, not mail — there is no fresh context yet", dir)
	}
	if len(deps.modelSet) != 1 || deps.modelSet[0] != "dvalin=big-model" {
		t.Errorf("modelSet = %v, want dvalin retiered to big-model", deps.modelSet)
	}

	// The restart landed; the new pod's first ask sees the fresh context mail belongs in.
	deps.currentModel = "big-model"
	dir, err = e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "unread") {
		t.Errorf("directive = %q, want mail delivered into the restarted pod before the claim", dir)
	}
}

// TestMailDefersPastCompactionBetweenSubtasks is TestMailDefersPastCompactionOnAClaim's counterpart
// for a held feature (claimNextSubtask), the other half of the boundary this bug hit in practice.
func TestMailDefersPastCompactionBetweenSubtasks(t *testing.T) {
	const agent = "dain"
	root, _ := newWorkRepo(t, agent, "td-EPIC")
	st, err := store.Open(root + "/s.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: ".worktrees/" + agent}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-EPIC", Title: "a feature", Status: "open", Priority: "P1", Type: "epic"}); err != nil {
		t.Fatalf("seed feature: %v", err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-next", Title: "the next subtask", Status: "open", Priority: "P1", ParentID: "td-EPIC"}); err != nil {
		t.Fatalf("seed subtask cache: %v", err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-next", Title: "the next subtask", Status: "open"}); err != nil {
		t.Fatalf("seed subtask: %v", err)
	}
	if err := ps.SetParent("td-next", "td-EPIC"); err != nil {
		t.Fatalf("set parent: %v", err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	mailID := addUnreadMail(t, ps, agent)

	deps := &stubDeps{root: root, ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e := New(st, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", agent)
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "unread") {
		t.Errorf("directive = %q, want mail delivered into the just-compacted context, not the subtask", dir)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != agent {
		t.Errorf("compacted = %v, want exactly one Compact(%s) fired ahead of the mail", deps.compacted, agent)
	}
	if held, _ := ps.GetState(agent); held.Task == "td-next" {
		t.Error("the next subtask must not be claimed until the mail is read")
	}

	if err := ps.MarkMailRead(mailID); err != nil {
		t.Fatal(err)
	}
	dir, err = e.AgentDirective(context.Background(), "repo", agent)
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "td-next") {
		t.Errorf("directive = %q, want the next subtask claimed now the mailbox is empty", dir)
	}
}

// TestMailOutranksAFeatureWithNothingLeftOpen: no subtask waiting and nothing gated means there is
// no operation for mail to wait out, so it must still be delivered ahead of whatever containerNext
// reports (done or blocked) — this is the ordinary "mail outranks blocking" rule, not the exception.
func TestMailOutranksAFeatureWithNothingLeftOpen(t *testing.T) {
	const agent = "dain"
	root, _ := newWorkRepo(t, agent, "td-EPIC")
	st, err := store.Open(root + "/s.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: ".worktrees/" + agent}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-EPIC", Title: "a feature", Status: "open", Priority: "P1", Type: "epic"}); err != nil {
		t.Fatalf("seed feature: %v", err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	addUnreadMail(t, ps, agent)

	e := New(st, &stubDeps{root: root})
	dir, claimed, err := e.claimNextSubtask("repo", agent, "td-EPIC")
	if err != nil {
		t.Fatalf("claimNextSubtask: %v", err)
	}
	if !claimed || !strings.Contains(dir, "unread") {
		t.Errorf("directive = %q, claimed = %v, want mail delivered rather than left blocking", dir, claimed)
	}
}

// TestMailDefersPastCompactionOnAReviewClaim mirrors the worker case for a reviewer about to be
// handed a new PR rather than a new task — the same gate, following the same rule.
func TestMailDefersPastCompactionOnAReviewClaim(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := reviewerWithUnclaimedReview(t, deps)
	mailID := addUnreadMail(t, ps, "rune")

	dir, err := e.AgentDirective(context.Background(), "repo", "rune")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "unread") {
		t.Errorf("directive = %q, want mail delivered into the just-compacted context, not the review", dir)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != "rune" {
		t.Errorf("compacted = %v, want exactly one Compact(rune) fired ahead of the mail", deps.compacted)
	}
	if held, _ := ps.ReviewingPR("rune"); held != "" {
		t.Errorf("ReviewingPR = %q, the review must not be claimed until the mail is read", held)
	}

	if err := ps.MarkMailRead(mailID); err != nil {
		t.Fatal(err)
	}
	dir, err = e.AgentDirective(context.Background(), "repo", "rune")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "pr-1") {
		t.Errorf("directive = %q, want the review claimed now the mailbox is empty", dir)
	}
}
