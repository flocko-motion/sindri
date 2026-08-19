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

// TestMailSurvivesAnArmedClear is the OBSERVED bug: an armed clear is about to wipe the agent's
// context, so mail delivered ahead of it would be marked read and then discarded along with the
// context that held it — worse than never having shown it, since an unread message survives the
// clear and is re-served. Firing the clear (fireClearIfArmed, checked ahead of any claim) never
// reaches the mail check at all; only once it has landed does mail get a look.
func TestMailSurvivesAnArmedClear(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)
	mailID := addUnreadMail(t, ps, "dvalin")
	a, _, _ := ps.GetAgent("dvalin")
	a.ClearArmed = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}

	fired, err := e.fireClearIfArmed("repo", "dvalin")
	if err != nil || !fired {
		t.Fatalf("fireClearIfArmed = (%v, %v), want fired", fired, err)
	}
	if n, _ := ps.UnreadMailCount("dvalin"); n != 1 {
		t.Errorf("unread = %d, the message must survive since it was never shown", n)
	}

	// The real FireClear spends the arming as its first act; the fake only records the call, so the
	// test spends it by hand — the fresh context (a real clear's own doing) now asks again.
	a.ClearArmed = false
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
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

// TestMailOutranksCompactionOnAClaim: mail is checked before the claim that would otherwise trigger
// a compaction, so it wins outright in the very first ask — there is no "defer past compaction and
// pick it up on a later one" dance, because claim and compaction now happen together in one pass and
// mail is what decides whether that pass runs at all.
func TestMailOutranksCompactionOnAClaim(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := idleWorkerWithOpenTask(t, deps)
	mailID := addUnreadMail(t, ps, "dvalin")

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "unread") {
		t.Errorf("directive = %q, want mail delivered ahead of the claim", dir)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — mail wins before the claim that would trigger it", deps.compacted)
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
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — the claim fires the compaction it also triggers", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "td-abc123" {
		t.Errorf("state.Task = %q, want td-abc123 — claimed even though this ask doesn't say so", st.Task)
	}
	if len(deps.compacted) != 1 {
		t.Errorf("compacted = %v, want exactly one fire, alongside the claim", deps.compacted)
	}
	if len(deps.compactedWith) != 1 || !strings.Contains(deps.compactedWith[0], "td-abc123") {
		t.Errorf("compactedWith = %v, want the claimed directive queued behind /compact", deps.compactedWith)
	}
}

// TestMailOutranksAModelChange mirrors TestMailOutranksCompactionOnAClaim for a task whose tier
// needs a different model: mail is checked (and wins) before the claim that would trigger the
// switch, so a model change never fires while there is unread mail sitting on the claim it prepares.
func TestMailOutranksAModelChange(t *testing.T) {
	deps := &stubDeps{tierModels: map[string]string{"mid": "big-model"}, currentModel: "small-model"}
	e, ps := idleWorkerWithOpenTask(t, deps)
	mailID := addUnreadMail(t, ps, "dvalin")

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "unread") {
		t.Errorf("directive = %q, want mail delivered ahead of the claim", dir)
	}
	if len(deps.modelSet) != 0 {
		t.Errorf("modelSet = %v, want none — mail wins before the claim that would trigger it", deps.modelSet)
	}

	if err := ps.MarkMailRead(mailID); err != nil {
		t.Fatal(err)
	}
	dir, err = e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — the claim fires the model switch it also triggers", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "td-abc123" {
		t.Errorf("state.Task = %q, want td-abc123 — claimed even though this ask doesn't say so", st.Task)
	}
	if len(deps.modelSet) != 1 || deps.modelSet[0] != "dvalin=big-model" {
		t.Errorf("modelSet = %v, want dvalin switched to big-model, alongside the claim", deps.modelSet)
	}
	if len(deps.modelSetWith) != 1 || !strings.Contains(deps.modelSetWith[0], "td-abc123") {
		t.Errorf("modelSetWith = %v, want the claimed directive queued as SetModel's next", deps.modelSetWith)
	}
}

// TestMailOutranksCompactionBetweenSubtasks is TestMailOutranksCompactionOnAClaim's counterpart for
// a held feature (claimNextSubtask), the other half of the boundary this bug hit in practice.
func TestMailOutranksCompactionBetweenSubtasks(t *testing.T) {
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
		t.Errorf("directive = %q, want mail delivered ahead of the claim", dir)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — mail wins before the claim that would trigger it", deps.compacted)
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
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — the claim fires the compaction it also triggers", dir)
	}
	if held, _ := ps.GetState(agent); held.Task != "td-next" {
		t.Errorf("state.Task = %q, want td-next — claimed even though this ask doesn't say so", held.Task)
	}
	if len(deps.compacted) != 1 {
		t.Errorf("compacted = %v, want exactly one fire, alongside the claim", deps.compacted)
	}
	if len(deps.compactedWith) != 1 || !strings.Contains(deps.compactedWith[0], "td-next") {
		t.Errorf("compactedWith = %v, want the next-subtask directive queued behind /compact", deps.compactedWith)
	}
}

// TestMailOutranksAFeatureWithNothingLeftOpen: no subtask waiting and nothing gated means there is
// no operation for mail to wait out, so it must still be delivered ahead of whatever
// claimNextSubtask reports (done or blocked) — this is the ordinary "mail outranks blocking" rule,
// not the exception.
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

// TestMailOutranksCompactionOnAReviewClaim mirrors the worker case for a reviewer about to be
// handed a new PR rather than a new task — the same gate, following the same rule.
func TestMailOutranksCompactionOnAReviewClaim(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := reviewerWithUnclaimedReview(t, deps)
	mailID := addUnreadMail(t, ps, "rune")

	dir, err := e.AgentDirective(context.Background(), "repo", "rune")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "unread") {
		t.Errorf("directive = %q, want mail delivered ahead of the claim", dir)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — mail wins before the claim that would trigger it", deps.compacted)
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
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — the claim fires the compaction it also triggers", dir)
	}
	if held, _ := ps.ReviewingPR("rune"); held != "pr-1" {
		t.Errorf("ReviewingPR = %q, want pr-1 — claimed even though this ask doesn't say so", held)
	}
	if len(deps.compacted) != 1 {
		t.Errorf("compacted = %v, want exactly one fire, alongside the claim", deps.compacted)
	}
	if len(deps.compactedWith) != 1 || !strings.Contains(deps.compactedWith[0], "pr-1") {
		t.Errorf("compactedWith = %v, want the review directive queued behind /compact", deps.compactedWith)
	}
}
