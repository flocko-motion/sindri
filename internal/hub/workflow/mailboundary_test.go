package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// addUnreadMail seeds one unread message for an agent and returns its id.
func addUnreadMail(t *testing.T, ps *store.ProjectStore, agent string) int64 {
	t.Helper()
	m, err := ps.AddMail(agent, "hub", "[hub] something happened", false, 0)
	if err != nil {
		t.Fatalf("AddMail: %v", err)
	}
	return m.ID
}

// deliveredContaining reports whether any message the stub was handed carries needle.
func deliveredContaining(d *stubDeps, needle string) bool {
	for _, text := range d.injectedText {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

// assertMailRead fails unless the agent's mailbox is now empty of unread mail — the ask itself is
// what marks it read, so every non-deferred directive must leave none behind.
func assertMailRead(t *testing.T, ps *store.ProjectStore, agent string) {
	t.Helper()
	if n, _ := ps.UnreadMailCount(agent); n != 0 {
		t.Errorf("unread(%s) = %d, want the ask to have marked it read", agent, n)
	}
}

// TestMailOutranksRetirement: a retired agent is told, in as many words, not to ask again — mail is
// served alongside that notice in the SAME call regardless, since retirement is not an operation
// that would discard the context mail is read into.
func TestMailOutranksRetirement(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)
	addUnreadMail(t, ps, "dvalin")
	a, _, _ := ps.GetAgent("dvalin")
	a.Retired = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "something happened") {
		t.Errorf("directive = %q, want the mail served inline", dir)
	}
	if !strings.Contains(dir, "retired") {
		t.Errorf("directive = %q, want the retirement notice alongside it", dir)
	}
	assertMailRead(t, ps, "dvalin")
}

// TestMailDefersPastAFullContextsClear is the automatic counterpart to TestMailDefersPastAnArmedClear
// below: a full worker's own ask claims its next task and then fires a clear to prepare it, discarding
// the very context mail would be read into — so mail is deferred here too, exactly as an armed clear
// defers it, served once the fresh context asks again.
func TestMailDefersPastAFullContextsClear(t *testing.T) {
	deps := &stubDeps{ctxTokens: 900_000, ctxWindow: 1_000_000, ctxOK: true}
	e, ps := idleWorkerWithOpenTask(t, deps)
	addUnreadMail(t, ps, "dvalin")

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — the clear must fire before anything else is served", dir)
	}
	if n, _ := ps.UnreadMailCount("dvalin"); n != 1 {
		t.Errorf("unread = %d, the message must survive since it was never shown", n)
	}
}

// TestMailDefersPastAnArmedClear is a genuine deferral: an armed clear is about to wipe the agent's
// context, so serving mail ahead of it would mark it read and then discard it along with the
// context that held it — worse than never showing it, since an unread message survives the clear
// and is served once there. The clear fires alone; mail lands only on the ask that follows.
func TestMailDefersPastAnArmedClear(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)
	addUnreadMail(t, ps, "dvalin")
	a, _, _ := ps.GetAgent("dvalin")
	a.ClearArmed = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}

	fired, err := e.fireClearIfArmed(t.Context(), "repo", "dvalin")
	if err != nil || !fired {
		t.Fatalf("fireClearIfArmed = (%v, %v), want the armed clear fired", fired, err)
	}
	if n, _ := ps.UnreadMailCount("dvalin"); n != 1 {
		t.Errorf("unread = %d, the message must survive since it was never shown", n)
	}

	// The clear has landed (a real one flips the flag itself); the fresh context's next ask gets the
	// mail AND the claim together — no separate round trip needed to see both.
	a.ClearArmed = false
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "something happened") {
		t.Errorf("directive = %q, want mail delivered into the fresh context", dir)
	}
	if !strings.Contains(dir, "td-abc123") {
		t.Errorf("directive = %q, want the task claimed in the same call", dir)
	}
	assertMailRead(t, ps, "dvalin")
}

// TestMailDefersPastAnArmedClearForAReviewer is TestMailDefersPastAnArmedClear's counterpart for
// reviewDirective: firing the clear must answer DirPreparing, not carry on and serve the review in
// the same reply — mailDeferred defers on DirPreparing alone, so any other answer marks the unread
// mail read and renders it into the very context the clear has just discarded.
func TestMailDefersPastAnArmedClearForAReviewer(t *testing.T) {
	deps := &stubDeps{}
	e, ps := reviewerWithUnclaimedReview(t, deps)
	addUnreadMail(t, ps, "rune")
	a, _, _ := ps.GetAgent("rune")
	a.ClearArmed = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}

	dir, err := e.AgentDirective(context.Background(), "repo", "rune")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — the review claim waits for the clear to land", dir)
	}
	if n, _ := ps.UnreadMailCount("rune"); n != 1 {
		t.Errorf("unread = %d, the message must survive since it was never shown", n)
	}
}

// TestMailDefersPastCompactionOnAClaim: the claim comes first (-> claimNext), so the task is
// already the agent's by the time prepareAssignment fires a due compaction — but the reply for
// THIS call is DirPreparing, not the claim text, so mail waits for the ask that actually reads it
// rather than being read into a reply nobody's session treats as the fresh context.
func TestMailDefersPastCompactionOnAClaim(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := idleWorkerWithOpenTask(t, deps)
	addUnreadMail(t, ps, "dvalin")

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — the claim fires the compaction it also triggers", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "td-abc123" {
		t.Errorf("state.Task = %q, want td-abc123 — claimed even though this ask doesn't say so", st.Task)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != "dvalin" {
		t.Errorf("compacted = %v, want exactly one Compact(dvalin) fired alongside the claim", deps.compacted)
	}
	if len(deps.injectedText) != 1 || !strings.Contains(deps.injectedText[0], "td-abc123") {
		t.Errorf("injectedText = %v, want the claimed directive delivered after the compaction", deps.injectedText)
	}
	if n, _ := ps.UnreadMailCount("dvalin"); n != 1 {
		t.Errorf("unread = %d, the message must survive since it was never shown", n)
	}

	// The queued /compact has landed and the agent is now "working" on what it already holds; the
	// next ask reads the mail that waited it out.
	deps.ctxTokens = 2_000
	dir, err = e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "something happened") {
		t.Errorf("directive = %q, want mail delivered once the compaction has landed", dir)
	}
	assertMailRead(t, ps, "dvalin")
}

// retieringDeps wraps stubDeps so SetModel takes effect within the same call, the way a real model
// switch changes what CurrentModel reports even though the pod itself restarts in the background.
type retieringDeps struct {
	*stubDeps
}

func (d retieringDeps) SetModel(ctx context.Context, project, name, model string) error {
	if err := d.stubDeps.SetModel(ctx, project, name, model); err != nil {
		return err
	}
	d.stubDeps.currentModel = model
	return nil
}

// TestMailDefersPastAModelChange: a retiering restart is itself a fresh-context boundary — SetModel
// takes effect within this very call (as CurrentModel now reports), so there is nothing yet to read
// mail into. The claimed directive queues behind it as SetModel's own next, and mail lands on the
// next ask, in the pod that comes back up under the new model.
func TestMailDefersPastAModelChange(t *testing.T) {
	const agent = "dvalin"
	root, _ := newWorkRepo(t, agent, "seed")
	base := &stubDeps{root: root, tierModels: map[string]string{"mid": "big-model"}, currentModel: "small-model"}
	deps := retieringDeps{base}
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
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-abc123", Title: "a task", Status: "open", Priority: "P2"}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatalf("set state: %v", err)
	}
	e := New(st, deps)
	addUnreadMail(t, ps, agent)

	dir, err := e.AgentDirective(context.Background(), "repo", agent)
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — the claim fires the model switch it also triggers", dir)
	}
	if st, _ := ps.GetState(agent); st.Task != "td-abc123" {
		t.Errorf("state.Task = %q, want td-abc123 — claimed even though this ask doesn't say so", st.Task)
	}
	if len(base.modelSet) != 1 || base.modelSet[0] != agent+"=big-model" {
		t.Errorf("modelSet = %v, want %s switched to big-model", base.modelSet, agent)
	}
	if len(base.injectedText) != 1 || !strings.Contains(base.injectedText[0], "td-abc123") {
		t.Errorf("injectedText = %v, want the claimed directive delivered once the switch answered", base.injectedText)
	}
	if n, _ := ps.UnreadMailCount(agent); n != 1 {
		t.Errorf("unread = %d, the message must survive since it was never shown", n)
	}

	// The restart landed; the new pod's first ask gets the mail.
	dir, err = e.AgentDirective(context.Background(), "repo", agent)
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "something happened") {
		t.Errorf("directive = %q, want mail delivered into the restarted pod", dir)
	}
	assertMailRead(t, ps, agent)
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
	if err := ps.SetState(store.AgentState{Agent: agent, Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatalf("set state: %v", err)
	}
	addUnreadMail(t, ps, agent)

	deps := &stubDeps{root: root, ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e := New(st, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", agent)
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — the claim fires the compaction it also triggers", dir)
	}
	if held, _ := ps.GetState(agent); held.Task != "td-next" {
		t.Errorf("state.Task = %q, want td-next — claimed even though this ask doesn't say so", held.Task)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != agent {
		t.Errorf("compacted = %v, want exactly one Compact(%s) fired alongside the claim", deps.compacted, agent)
	}
	if len(deps.injectedText) != 1 || !strings.Contains(deps.injectedText[0], "td-next") {
		t.Errorf("injectedText = %v, want the next-subtask directive delivered after the compaction", deps.injectedText)
	}
	if n, _ := ps.UnreadMailCount(agent); n != 1 {
		t.Errorf("unread = %d, the message must survive since it was never shown", n)
	}

	// The queued /compact has landed; the next ask reads the mail that waited it out.
	deps.ctxTokens = 2_000
	dir, err = e.AgentDirective(context.Background(), "repo", agent)
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "something happened") {
		t.Errorf("directive = %q, want mail delivered once the compaction has landed", dir)
	}
	assertMailRead(t, ps, agent)
}

// TestMailOutranksAFeatureWithNothingLeftOpen: no subtask waiting and nothing gated is an ordinary
// directive (DirContainerDone), not a deferral case, so mail rides along with it same as any other.
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
	if err := ps.SetState(store.AgentState{Agent: agent, Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatalf("set state: %v", err)
	}
	addUnreadMail(t, ps, agent)

	e := New(st, &stubDeps{root: root})
	dir, err := e.AgentDirective(context.Background(), "repo", agent)
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "something happened") {
		t.Errorf("directive = %q, want mail delivered alongside the feature's own directive", dir)
	}
	assertMailRead(t, ps, agent)
}

// TestSeveralUnreadMessagesAllServeAndAllMarkRead: DirMail renders every unread message, not just
// the first, oldest first, and the ask marks every one of them read in the same pass.
func TestSeveralUnreadMessagesAllServeAndAllMarkRead(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)
	if _, err := ps.AddMail("dvalin", "hub", "[hub] first thing", false, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddMail("dvalin", "reviewer", "[reviewer] second thing", false, 0); err != nil {
		t.Fatal(err)
	}

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	firstAt, secondAt := strings.Index(dir, "first thing"), strings.Index(dir, "second thing")
	if firstAt < 0 || secondAt < 0 {
		t.Fatalf("both messages should be served, got: %q", dir)
	}
	if firstAt > secondAt {
		t.Errorf("oldest should come first, got: %q", dir)
	}
	if !strings.Contains(dir, "td-abc123") {
		t.Errorf("the real directive should still follow both: %q", dir)
	}
	assertMailRead(t, ps, "dvalin")
}

// TestMailDefersPastCompactionOnAReviewClaim mirrors the worker case for a reviewer about to be
// handed a new PR rather than a new task — the same rule, following the same shape.
func TestMailDefersPastCompactionOnAReviewClaim(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := reviewerWithUnclaimedReview(t, deps)
	addUnreadMail(t, ps, "rune")

	dir, err := e.AgentDirective(context.Background(), "repo", "rune")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — the claim fires the compaction it also triggers", dir)
	}
	if held, _ := ps.ReviewingPR("rune"); held != "pr-1" {
		t.Errorf("ReviewingPR = %q, want pr-1 — claimed even though this ask doesn't say so", held)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != "rune" {
		t.Errorf("compacted = %v, want exactly one Compact(rune) fired alongside the claim", deps.compacted)
	}
	// Searched rather than counted: assignReview pushes its own "you have a review" note from a
	// goroutine, so the number of deliveries here is not this test's to fix.
	if !deliveredContaining(deps, "check the gate") {
		t.Errorf("injectedText = %v, want the review directive delivered after the compaction", deps.injectedText)
	}
	if n, _ := ps.UnreadMailCount("rune"); n != 1 {
		t.Errorf("unread = %d, the message must survive since it was never shown", n)
	}

	// The queued /compact has landed; the next ask reads the mail that waited it out.
	deps.ctxTokens = 2_000
	dir, err = e.AgentDirective(context.Background(), "repo", "rune")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "something happened") {
		t.Errorf("directive = %q, want mail delivered once the compaction has landed", dir)
	}
	assertMailRead(t, ps, "rune")
}
