package workflow

import (
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
}
