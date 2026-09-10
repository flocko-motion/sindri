package fleet

import (
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/reviewer"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// poolFixture seeds an open, unclaimed review in "repo" plus whatever reviewers the test wants, in
// either project.
func poolFixture(t *testing.T) (*store.Store, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "td-1", Agent: "bombur", Branch: "sd-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddReview("pr-1", "review it"); err != nil {
		t.Fatal(err)
	}
	return st, ps
}

// TestIdleReviewerPrefersALocalOneOverTheGlobalPool: a repo that keeps its own reviewer expects it
// used, so a free local reviewer wins even when the global pool also has one sitting idle.
func TestIdleReviewerPrefersALocalOneOverTheGlobalPool(t *testing.T) {
	st, ps := poolFixture(t)
	if err := ps.PutAgent(store.Agent{Name: "fili", Role: "reviewer", Workspace: ".worktrees/fili"}); err != nil {
		t.Fatal(err)
	}
	if err := st.For(api.GlobalProject).PutAgent(store.Agent{Name: "ori", Role: "reviewer", Workspace: "ori"}); err != nil {
		t.Fatal(err)
	}
	e := newEngine(t, st, &flowtest.Hub{Root: t.TempDir(), Alive: true})

	got, err := e.prAct().IdleReviewer("repo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "fili" {
		t.Errorf("IdleReviewer = %q, want the local reviewer fili", got)
	}
}

// TestIdleReviewerFallsBackToTheGlobalPool: a project with no free reviewer of its own can still
// draw on one living in api.GlobalProject — the capability a project-scoped roster never offered.
func TestIdleReviewerFallsBackToTheGlobalPool(t *testing.T) {
	st, ps := poolFixture(t)
	if err := ps.PutAgent(store.Agent{Name: "fili", Role: "reviewer", Workspace: ".worktrees/fili"}); err != nil {
		t.Fatal(err)
	}
	if err := st.For(api.GlobalProject).PutAgent(store.Agent{Name: "ori", Role: "reviewer", Workspace: "ori"}); err != nil {
		t.Fatal(err)
	}
	e := newEngine(t, st, &flowtest.Hub{Root: t.TempDir(), Alive: true, Busy: map[string]bool{"fili": true}})

	got, err := e.prAct().IdleReviewer("repo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ori" {
		t.Errorf("IdleReviewer = %q, want the global reviewer ori once the local one is busy", got)
	}
}

// TestIdleReviewerSeesAGlobalReviewerBusyInAnotherProject: "what is this reviewer reviewing" has to
// be a fleet-wide question for a api.GlobalProject reviewer, or a project-scoped read reports it free
// while it is plainly busy on a PR the current project never touches.
func TestIdleReviewerSeesAGlobalReviewerBusyInAnotherProject(t *testing.T) {
	st, _ := poolFixture(t)
	gs := st.For(api.GlobalProject)
	if err := gs.PutAgent(store.Agent{Name: "ori", Role: "reviewer", Workspace: "ori"}); err != nil {
		t.Fatal(err)
	}
	other := st.For("other-repo")
	if err := other.PutPR(store.PR{ID: "pr-elsewhere", Task: "td-2", Agent: "dain", Branch: "sd-2", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	rid, err := other.AddReview("pr-elsewhere", "check it")
	if err != nil {
		t.Fatal(err)
	}
	if err := other.AssignReview(rid, "ori"); err != nil {
		t.Fatal(err)
	}
	e := newEngine(t, st, &flowtest.Hub{Root: t.TempDir(), Alive: true, Projects: []store.Project{{Tag: "other-repo"}}})

	got, err := e.prAct().IdleReviewer("repo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("IdleReviewer = %q, want none — the only reviewer is busy in another project", got)
	}
}

// TestAssignReviewResolvesAGlobalReviewersOwnRecord is the bug fixed at the assignment itself: a
// api.GlobalProject reviewer's roster row, state and notes live under api.GlobalProject, never the PR's
// project, and the old lookup (the PR's own store) failed "not on roster" on every review.
func TestAssignReviewResolvesAGlobalReviewersOwnRecord(t *testing.T) {
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
	run(root, "branch", "sd-1")
	wt := filepath.Join(root, "ori")
	run(root, "worktree", "add", "-q", "--detach", wt, "main")

	st, ps := poolFixture(t)
	if err := st.For(api.GlobalProject).PutAgent(store.Agent{Name: "ori", Role: "reviewer", Workspace: "ori"}); err != nil {
		t.Fatal(err)
	}
	e := newEngine(t, st, &flowtest.Hub{Root: root, Alive: true})

	rid, err := ps.AddReview("pr-1", "look again")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.prAct().AssignReview(t.Context(), "repo", rid, "pr-1", "ori", "look again"); err != nil {
		t.Fatal(err)
	}

	entries, err := ps.PREvents("pr-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range entries {
		if ev.Type == "checkout-failed" {
			t.Errorf("checkout-failed logged (%s) — its own record, found under api.GlobalProject, should have checked out clean", ev.Payload)
		}
	}
	// The hand-over assigns the review; where the reviewer STANDS is its own map's to write, under
	// its own home rather than the PR's project.
	e.Look(api.GlobalProject, "ori")
	gstate, err := st.For(api.GlobalProject).GetState("ori")
	if err != nil {
		t.Fatal(err)
	}
	if gstate.Phase != reviewer.Reviewing {
		t.Errorf("ori's phase under api.GlobalProject = %q, want reviewing", gstate.Phase)
	}
	if repoState, err := ps.GetState("ori"); err == nil && repoState.Phase == reviewer.Reviewing {
		t.Errorf("ori's state was also written under the PR's project — it belongs to ori's own home only")
	}
}

// TestAssignReviewMaterialisesAGlobalReviewersWorkspaceAsPlainFiles: a api.GlobalProject reviewer has no
// git of its own, so its fixed workspace gets the PR's tree exported as plain files rather than
// checked out in place — no .git, and whatever the last review left there is gone.
func TestAssignReviewMaterialisesAGlobalReviewersWorkspaceAsPlainFiles(t *testing.T) {
	repoRoot := t.TempDir()
	if err := exec.Command("git", "init", "-q", "-b", "main", repoRoot).Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", repoRoot}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repoRoot, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "base")
	run("checkout", "-q", "-b", "sd-1")
	if err := os.WriteFile(filepath.Join(repoRoot, "changed.txt"), []byte("pr content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "pr commit")
	run("checkout", "-q", "main")

	st, ps := poolFixture(t)
	if err := st.For(api.GlobalProject).PutAgent(store.Agent{Name: "ori", Role: "reviewer", Workspace: "ori"}); err != nil {
		t.Fatal(err)
	}
	// A file the LAST review left, that this PR's tree does not carry.
	wt := filepath.Join(repoRoot, "ori")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "leftover.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := newEngine(t, st, &flowtest.Hub{Root: repoRoot, Alive: true})
	rid, err := ps.AddReview("pr-1", "look")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.prAct().AssignReview(t.Context(), "repo", rid, "pr-1", "ori", "look"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(wt, "changed.txt")); err != nil {
		t.Errorf("the PR's own file should be materialised: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, "leftover.txt")); !os.IsNotExist(err) {
		t.Errorf("the last review's leftover file should be gone, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, ".git")); !os.IsNotExist(err) {
		t.Error("a global reviewer's workspace must hold no .git — plain files, not a repository")
	}
}
