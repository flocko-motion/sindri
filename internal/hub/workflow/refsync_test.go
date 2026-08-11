package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// syncFixture is a repo pinned to "main" with one agent on its own worktree branch, plus the engine
// and a stub that records what each agent was told.
type syncFixture struct {
	e     *Engine
	deps  *stubDeps
	ps    *store.ProjectStore
	root  string
	wt    string
	runIn func(dir string, args ...string)
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	runIn := func(dir string, args ...string) {
		t.Helper()
		if out, e := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); e != nil {
			t.Fatalf("git -C %s %v: %s", dir, args, out)
		}
	}
	runIn(root, "init", "-q", "-b", "main")
	runIn(root, "config", "user.email", "t@t")
	runIn(root, "config", "user.name", "t")
	if e := os.WriteFile(filepath.Join(root, "base.txt"), []byte("one\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	runIn(root, "add", "-A")
	runIn(root, "commit", "-qm", "base")
	writeRef(t, root, "main") // pin it, so the checkout's state is irrelevant

	wt := filepath.Join(root, ".worktrees", "eitri")
	runIn(root, "worktree", "add", "-q", "-b", "work", wt)
	if e := os.WriteFile(filepath.Join(wt, "mine.txt"), []byte("my work\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	runIn(wt, "add", "-A")
	runIn(wt, "commit", "-qm", "my own work")

	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker", Workspace: ".worktrees/eitri"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Branch: "work", Phase: "working"}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	deps := &stubDeps{root: root}
	return &syncFixture{e: New(st, deps), deps: deps, ps: ps, root: root, wt: wt, runIn: runIn}
}

// moveReference adds a commit to main, advancing it.
func (f *syncFixture) moveReference(t *testing.T, body, msg string) {
	t.Helper()
	if e := os.WriteFile(filepath.Join(f.root, "base.txt"), []byte(body), 0o644); e != nil {
		t.Fatal(e)
	}
	f.runIn(f.root, "commit", "-aqm", msg)
}

// told returns everything injected into the agent since the fixture was made.
func (f *syncFixture) told() string { return strings.Join(f.deps.injectedText, "\n") }

// TestSyncReferenceFirstLookSaysNothing: with no remembered tip nothing can be said to have moved,
// so a hub restart must not tell every agent the world changed.
func TestSyncReferenceFirstLookSaysNothing(t *testing.T) {
	f := newSyncFixture(t)
	if err := f.e.SyncReference("proj"); err != nil {
		t.Fatalf("SyncReference: %v", err)
	}
	if got := f.told(); got != "" {
		t.Errorf("the first look must be silent, said: %q", got)
	}
	// And a second look with nothing moved is silent too.
	if err := f.e.SyncReference("proj"); err != nil {
		t.Fatalf("SyncReference: %v", err)
	}
	if got := f.told(); got != "" {
		t.Errorf("an unmoved reference must be silent, said: %q", got)
	}
}

// TestSyncReferenceAdvanceRebasesAndReports: a fast-forward advance is the safe case — replay the
// agent's work onto it and name what arrived, so it can re-check anything built on the change.
func TestSyncReferenceAdvanceRebasesAndReports(t *testing.T) {
	f := newSyncFixture(t)
	if err := f.e.SyncReference("proj"); err != nil { // record the starting tip
		t.Fatalf("SyncReference: %v", err)
	}
	f.moveReference(t, "one\ntwo\n", "upstream adds a line")
	if err := f.e.SyncReference("proj"); err != nil {
		t.Fatalf("SyncReference: %v", err)
	}
	got := f.told()
	if !strings.Contains(got, "moved on") || !strings.Contains(got, "upstream adds a line") {
		t.Fatalf("the agent should be told what arrived, said: %q", got)
	}
	if strings.Contains(got, "main") {
		t.Errorf("the reply names the reference branch: %q", got)
	}
	// Its work was actually replayed: the branch now contains the new base commit.
	out, err := exec.Command("git", "-C", f.wt, "log", "--format=%s").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"my own work", "upstream adds a line"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("the agent's branch should carry %q after the rebase, got:\n%s", want, out)
		}
	}
}

// TestSyncReferenceRewriteWarnsAndLeavesTheBranchAlone is the case that cost a day: history was
// replaced, so the agent's CONCLUSIONS are stale even though its commits are fine. It must be told
// that plainly, and its branch must not be replayed under it while it may be mid-edit.
func TestSyncReferenceRewriteWarnsAndLeavesTheBranchAlone(t *testing.T) {
	f := newSyncFixture(t)
	if err := f.e.SyncReference("proj"); err != nil {
		t.Fatalf("SyncReference: %v", err)
	}
	// The REMEMBERED tip must be the one that gets rewritten. Amending a commit made after it
	// would leave it an ancestor still — an advance, correctly, not a rewrite.
	f.moveReference(t, "one\ntwo\n", "will be rewritten")
	if err := f.e.SyncReference("proj"); err != nil { // remember the doomed commit
		t.Fatalf("SyncReference: %v", err)
	}
	f.deps.injectedText = nil // that pass was an ordinary advance; assert on what follows
	before, err := exec.Command("git", "-C", f.wt, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	// Replace that commit, so the tip the hub remembers is no longer in the history at all.
	f.runIn(f.root, "commit", "-q", "--amend", "-m", "rewritten history")

	if err := f.e.SyncReference("proj"); err != nil {
		t.Fatalf("SyncReference: %v", err)
	}
	got := f.told()
	if !strings.Contains(got, "REWRITTEN") {
		t.Fatalf("a rewrite must be named as one, said: %q", got)
	}
	if !strings.Contains(got, "code you don't own") {
		t.Errorf("the warning must point at stale conclusions, not just commits: %q", got)
	}
	after, err := exec.Command("git", "-C", f.wt, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a rewrite must not move the agent's branch — it rebases when it is ready")
	}
}

// TestSyncReferenceLeavesABranchUnderReviewAlone: an advance must not move a submitted branch, or
// the reviewer's diff shifts under it. The merge rebases it when the time comes.
func TestSyncReferenceLeavesABranchUnderReviewAlone(t *testing.T) {
	f := newSyncFixture(t)
	if err := f.ps.SetState(store.AgentState{Agent: "eitri", Branch: "work", Phase: "submitted"}); err != nil {
		t.Fatal(err)
	}
	if err := f.e.SyncReference("proj"); err != nil {
		t.Fatalf("SyncReference: %v", err)
	}
	before, err := exec.Command("git", "-C", f.wt, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	f.moveReference(t, "one\ntwo\n", "upstream moves while under review")
	if err := f.e.SyncReference("proj"); err != nil {
		t.Fatalf("SyncReference: %v", err)
	}
	after, err := exec.Command("git", "-C", f.wt, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a branch under review must not be rebased out from under its reviewer")
	}
	if got := f.told(); got != "" {
		t.Errorf("an advance during review needs no message — the merge handles it: %q", got)
	}
	// The skip is silent to the agent, but must not be silent in the log — that is the whole task.
	log := agentLog(t, f.ps, "eitri")
	if !strings.Contains(log, "reference-review-skip") {
		t.Errorf("the under-review skip left no log entry: %q", log)
	}
	if !strings.Contains(log, "1 commit(s) behind") {
		t.Errorf("the skip should record how far behind the agent now is, got %q", log)
	}
}

// TestUnderReviewSkipReportsStandingDriftAcrossSweeps is the case that actually distinguishes a
// standing count from a per-move delta: three sweeps, each arriving one commit, with no rebase
// between (the agent is never moved while under review). A delta of "this move" would read "1
// commit(s) behind" three times on a branch that is really 3 behind by the third sweep — the exact
// failure the count exists to avoid. The standing figure (branch vs base) must grow 1, 2, 3.
func TestUnderReviewSkipReportsStandingDriftAcrossSweeps(t *testing.T) {
	f := newSyncFixture(t)
	if err := f.ps.SetState(store.AgentState{Agent: "eitri", Branch: "work", Phase: "submitted"}); err != nil {
		t.Fatal(err)
	}
	if err := f.e.SyncReference("proj"); err != nil {
		t.Fatalf("SyncReference: %v", err)
	}
	// "one\n" is the fixture's own initial content — starting there would be a no-op write with
	// nothing for `git commit -a` to pick up, so each body here is new relative to the last.
	for i, body := range []string{"one\ntwo\n", "one\ntwo\nthree\n", "one\ntwo\nthree\nfour\n"} {
		f.moveReference(t, body, fmt.Sprintf("sweep %d while under review", i+1))
		if err := f.e.SyncReference("proj"); err != nil {
			t.Fatalf("SyncReference (sweep %d): %v", i+1, err)
		}
	}
	evs, err := f.ps.Events("eitri", 0)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	var got []string
	for _, e := range evs {
		if e.Type == "reference-review-skip" {
			got = append(got, e.Payload)
		}
	}
	if len(got) != 3 {
		t.Fatalf("expected one skip logged per sweep, got %d: %v", len(got), got)
	}
	for i, want := range []string{"1 commit(s) behind", "2 commit(s) behind", "3 commit(s) behind"} {
		if !strings.Contains(got[i], want) {
			t.Errorf("sweep %d = %q, want it to contain %q — the standing drift, not this move's own delta",
				i+1, got[i], want)
		}
	}
}

// TestAdvanceIsSilentWhenNothingArrived is the fix: a reference that moved for somebody else must
// not produce a message that says nothing. The agent is still rebased — that is harmless and keeps
// the branch current — but its input stream is finite attention, and a message that reliably says
// nothing teaches it to skim the channel the hub also uses for verdicts and assignments.
func TestAdvanceIsSilentWhenNothingArrived(t *testing.T) {
	f := newSyncFixture(t)
	tip := strings.TrimSpace(gitOut(t, f.root, "rev-parse", "refs/heads/main"))
	a, ok, err := f.ps.GetAgent("eitri")
	if err != nil || !ok {
		t.Fatalf("get agent: ok=%v err=%v", ok, err)
	}
	// prevTip == tip: the range is empty, so nothing arrived for this branch.
	f.e.advanceAgent("proj", f.root, "main", tip, tip, a)
	if got := f.told(); got != "" {
		t.Errorf("a no-op advance spoke: %q", got)
	}
	// It is recorded, so the trail exists without interrupting the agent.
	if !strings.Contains(agentLog(t, f.ps, "eitri"), "reference-advanced-quiet") {
		t.Error("the quiet advance left no trail in the log")
	}
}

// TestAdvanceSpeaksWhenOnlyMergesArrived guards the trap in the obvious fix. LogRange passes
// --no-merges, so an advance made only of merge commits produces an EMPTY list while the reference
// genuinely moved. Keying silence on that list would mean the agent is never told.
func TestAdvanceSpeaksWhenOnlyMergesArrived(t *testing.T) {
	f := newSyncFixture(t)
	// Build a range whose only new commit is a merge: branch off main, commit there, then merge
	// back. Measured from the SIDE commit, the merge is the one thing main gained — the shape a
	// reference picks up whenever it was last seen at a commit the merge already contains.
	f.runIn(f.root, "checkout", "-q", "-b", "sidebranch")
	if e := os.WriteFile(filepath.Join(f.root, "side.txt"), []byte("side\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	f.runIn(f.root, "add", "-A")
	f.runIn(f.root, "commit", "-qm", "side work")
	prev := strings.TrimSpace(gitOut(t, f.root, "rev-parse", "HEAD")) // the side commit
	f.runIn(f.root, "checkout", "-q", "main")
	f.runIn(f.root, "merge", "--no-ff", "-q", "-m", "merge sidebranch", "sidebranch")
	tip := strings.TrimSpace(gitOut(t, f.root, "rev-parse", "refs/heads/main"))

	// The premise this test rests on: main moved, and the readable list really is empty.
	if prev == tip {
		t.Fatal("main did not move — the fixture proves nothing")
	}
	if lines := strings.TrimSpace(gitOut(t, f.root, "log", "--no-merges", "--format=%h", prev+".."+tip)); lines != "" {
		t.Fatalf("this advance was meant to be merge-only, got %q", lines)
	}
	a, ok, err := f.ps.GetAgent("eitri")
	if err != nil || !ok {
		t.Fatalf("get agent: ok=%v err=%v", ok, err)
	}
	f.e.advanceAgent("proj", f.root, "main", prev, tip, a)
	if got := f.told(); !strings.Contains(got, "moved on") {
		t.Errorf("a merge-only advance was silenced, so the agent never hears it: %q", got)
	}
}

// TestAdvanceSpeaksWhenItCannotTell: an unreadable range is not evidence that nothing arrived.
// Silence there would convert a git failure into an agent that never learns the reference moved,
// which is the worse of the two failures — so the doubt is resolved towards speaking.
func TestAdvanceSpeaksWhenItCannotTell(t *testing.T) {
	f := newSyncFixture(t)
	tip := strings.TrimSpace(gitOut(t, f.root, "rev-parse", "refs/heads/main"))
	a, ok, err := f.ps.GetAgent("eitri")
	if err != nil || !ok {
		t.Fatalf("get agent: ok=%v err=%v", ok, err)
	}
	// A prevTip no longer in the repo: rev-list fails rather than reporting zero.
	f.e.advanceAgent("proj", f.root, "main", "0000000000000000000000000000000000000000", tip, a)
	if got := f.told(); !strings.Contains(got, "moved on") {
		t.Errorf("an unreadable range was treated as 'nothing arrived': %q", got)
	}
	if !strings.Contains(agentLog(t, f.ps, "eitri"), "reference-count-failed") {
		t.Error("the failure that forced the fallback was not recorded")
	}
}

// gitOut runs a git command and returns its stdout.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git -C %s %v: %v", dir, args, err)
	}
	return string(out)
}

// agentLog joins an agent's recorded events, for asserting the trail a silent path still leaves.
func agentLog(t *testing.T, ps *store.ProjectStore, agent string) string {
	t.Helper()
	evs, err := ps.Events(agent, 0)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	var b strings.Builder
	for _, e := range evs {
		b.WriteString(e.Type + " " + e.Payload + "\n")
	}
	return b.String()
}

// TestAdvanceMeasuresTheTipTheMoveWasDecidedFrom: refwatch polls, so the branch can move again
// between the tip SyncReference compared and this rebase. Re-resolving the branch name here would
// report a range nobody decided on — naming commits the move was not about, and (when the branch
// moved back) inventing an advance out of nothing.
func TestAdvanceMeasuresTheTipTheMoveWasDecidedFrom(t *testing.T) {
	f := newSyncFixture(t)
	prev := strings.TrimSpace(gitOut(t, f.root, "rev-parse", "refs/heads/main"))
	f.moveReference(t, "two\n", "the commit the move was decided from")
	decided := strings.TrimSpace(gitOut(t, f.root, "rev-parse", "refs/heads/main"))
	// The branch keeps moving while the hub works through the roster.
	f.moveReference(t, "three\n", "landed after the decision")

	a, ok, err := f.ps.GetAgent("eitri")
	if err != nil || !ok {
		t.Fatalf("get agent: ok=%v err=%v", ok, err)
	}
	f.e.advanceAgent("proj", f.root, "main", prev, decided, a)

	got := f.told()
	if !strings.Contains(got, "the commit the move was decided from") {
		t.Errorf("the decided range was not reported: %q", got)
	}
	if strings.Contains(got, "landed after the decision") {
		t.Errorf("a commit from after the decision was reported as part of this move: %q", got)
	}
}
