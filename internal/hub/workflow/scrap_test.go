package workflow

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/store"
)

// stubDeps is a no-op workflow.Deps that records the agents it interrupts/injects —
// enough to drive ScrapPR without a real hub. (First workflow Engine test harness;
// extend as more Engine methods get covered.)
type stubDeps struct {
	root         string
	alive        bool
	interrupted  []string
	injected     []string
	injectedText []string // the message bodies too, for tests that assert what an agent was told
	ctxTokens    int      // TestContextFull* set these to simulate a worker's session usage
	ctxWindow    int      // 0 with ctxOK true means "measured, but the window is unknown"
	ctxOK        bool
	comments     map[string][]store.Comment // by task id, for the views that render a thread
	busy         map[string]bool            // agents mid-turn, so AgentIdle answers false for them
	posted       []store.Comment            // what the workflow wrote onto a task's thread (SourceRef holds the id)
}

func (d *stubDeps) ProjectRoot(string) string                   { return d.root }
func (d *stubDeps) ProjectConfig(string) (config.Config, error) { return config.Config{}, nil }
func (d *stubDeps) ArchitectureDoc(string) string               { return "" }
func (d *stubDeps) Container(_, name string) string             { return name }
func (d *stubDeps) Notify()                                     {}
func (d *stubDeps) InjectWhenReady(_, name, text string) error {
	d.injected = append(d.injected, name)
	d.injectedText = append(d.injectedText, text)
	return nil
}
func (d *stubDeps) Interrupt(_, name string) error {
	d.interrupted = append(d.interrupted, name)
	return nil
}
func (d *stubDeps) AgentAlive(_, _ string) bool               { return d.alive }
func (d *stubDeps) AgentIdle(_, name string) bool             { return !d.busy[name] }
func (d *stubDeps) SessionAlive(_, _ string) bool             { return false }
func (d *stubDeps) TaskComments(_, id string) []store.Comment { return d.comments[id] }
func (d *stubDeps) AddTaskComment(_, id, author, body string) error {
	d.posted = append(d.posted, store.Comment{SourceRef: id, Author: author, Body: body})
	return nil
}
func (d *stubDeps) Subscribe() (chan struct{}, func()) { return make(chan struct{}), func() {} }
func (d *stubDeps) KnownProjects() []store.Project     { return nil }
func (d *stubDeps) BrokkrBin() (string, error)         { return "", nil }
func (d *stubDeps) ContextUsage(_, _ string) (int, int, bool) {
	return d.ctxTokens, d.ctxWindow, d.ctxOK
}

// TestScrapPRStopsReviewer: scrapping a PR under review flips it to "scrapped",
// interrupts the reviewer and closes its open review record, so the reviewer no
// longer shows as reviewing a PR whose branch is gone. (ProjectRoot points at a
// non-repo, so the branch delete no-ops-with-a-log — the status flip must still land.)
func TestScrapPRStopsReviewer(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "td-1", Agent: "wrk", Branch: "td-1", Status: "submitted"}); err != nil {
		t.Fatal(err)
	}
	rid, err := ps.AddReview("pr-1", "check it")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.AssignReview(rid, "rev"); err != nil {
		t.Fatal(err)
	}
	if got, _ := ps.ReviewingPR("rev"); got != "pr-1" {
		t.Fatalf("precondition: reviewer should be reviewing pr-1, got %q", got)
	}

	e := New(st, &stubDeps{root: t.TempDir(), alive: true})
	if err := e.ScrapPR("repo", "pr-1"); err != nil {
		t.Fatalf("ScrapPR: %v", err)
	}

	if pr, _, _ := ps.GetPR("pr-1"); pr.Status != "scrapped" {
		t.Fatalf("PR status = %q, want scrapped", pr.Status)
	}
	if got, _ := ps.ReviewingPR("rev"); got != "" {
		t.Fatalf("reviewer should no longer be reviewing (verdict recorded), got %q", got)
	}
}

// TestScrapEmptiesAStandingBranch: a planner's branch is its HOME, created at launch and reused
// for every proposal. Deleting it meant detaching the worktree to free the name, which left the
// planner on a HEAD no branch held — it kept committing there and could never rebase again. Scrap
// must throw the plans away and leave the branch, attached, at the reference branch.
func TestScrapEmptiesAStandingBranch(t *testing.T) {
	root := t.TempDir()
	if err := exec.Command("git", "init", "-q", "-b", "main", root).Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	run := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run(root, "config", "user.email", "t@t")
	run(root, "config", "user.name", "t")
	run(root, "commit", "-q", "--allow-empty", "-m", "base")
	wt := filepath.Join(root, ".worktrees", "galar")
	run(root, "worktree", "add", "-q", "-b", "plan-galar", wt, "HEAD")
	run(wt, "commit", "-q", "--allow-empty", "-m", "a proposal the user does not want")

	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "galar", Role: "planner", Workspace: ".worktrees/galar"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-plan-galar", Task: "os-new", Agent: "galar", Branch: "plan-galar", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}

	e := New(st, &stubDeps{root: root, alive: false})
	if err := e.ScrapPR("proj", "pr-plan-galar"); err != nil {
		t.Fatalf("ScrapPR: %v", err)
	}

	// The branch still exists AND the worktree is still on it — the failure was losing both.
	if out, err := exec.Command("git", "-C", root, "rev-parse", "--verify", "refs/heads/plan-galar").Output(); err != nil {
		t.Fatalf("a standing branch must survive a scrap: %v %s", err, out)
	}
	if b, err := git.CurrentBranch(wt); err != nil || b != "plan-galar" {
		t.Fatalf("worktree must stay attached to plan-galar, got %q (%v)", b, err)
	}
	// And the plans are gone: the branch sits back on base.
	base, _ := exec.Command("git", "-C", root, "rev-parse", "main").Output()
	tip, _ := exec.Command("git", "-C", wt, "rev-parse", "HEAD").Output()
	if strings.TrimSpace(string(tip)) != strings.TrimSpace(string(base)) {
		t.Errorf("plans should be discarded: tip %s, base %s", tip, base)
	}
}

// TestDiscardPRReleasesItsAuthor: scrapping a PR on its own has no paired task close to free
// the author, so DiscardPR must — otherwise a planner whose proposal the user discards waits in
// "submitted" for a verdict on a PR that no longer exists.
func TestDiscardPRReleasesItsAuthor(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "galar", Role: "planner", Workspace: "."}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-os-new", Task: "os-new", Agent: "galar", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "galar", Task: "os-new", Phase: "submitted"}); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{root: root, alive: true}
	e := New(st, deps)

	if err := e.DiscardPR("proj", "pr-os-new"); err != nil {
		t.Fatalf("DiscardPR: %v", err)
	}
	if pr, _, _ := ps.GetPR("pr-os-new"); pr.Status != "scrapped" {
		t.Errorf("PR status = %q, want scrapped", pr.Status)
	}
	if got, _ := ps.GetState("galar"); got.Phase != "idle" {
		t.Errorf("author phase = %q, want idle — it must not wait on a PR that is gone", got.Phase)
	}
	if len(deps.injected) == 0 {
		t.Error("the author must be told its PR was scrapped")
	}
}

// TestDiscardPRLeavesAnUninvolvedAgentAlone: an author that has moved on must not be
// interrupted for a verdict it is not expecting.
func TestDiscardPRLeavesAnUninvolvedAgentAlone(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker", Workspace: "."}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-td-1", Task: "td-1", Agent: "eitri", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	// Already working something else — not waiting on this PR.
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: "td-9", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{root: root, alive: true}
	e := New(st, deps)

	if err := e.DiscardPR("proj", "pr-td-1"); err != nil {
		t.Fatalf("DiscardPR: %v", err)
	}
	if got, _ := ps.GetState("eitri"); got.Phase != "working" {
		t.Errorf("phase = %q, want working left untouched", got.Phase)
	}
	if len(deps.interrupted) != 0 {
		t.Errorf("an agent not waiting on the PR must not be interrupted, got %v", deps.interrupted)
	}
}
