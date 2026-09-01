package agent

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agentchan"
	"github.com/flo-at/sindri/internal/hub/store"
)

// newGitRepo makes a minimal repo with one commit — enough for prepareWorkspace's ordinary,
// non-GlobalProject path.
func newGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Skipf("git unavailable: %v: %s", err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	run("commit", "-q", "--allow-empty", "-m", "base")
	return dir
}

// TestPrepareWorkspaceForGlobalReviewerNeedsNoRepository: a GlobalProject pod mounts no repository
// at all — the fixed workspace directory just needs to exist, whatever root points at.
func TestPrepareWorkspaceForGlobalReviewerNeedsNoRepository(t *testing.T) {
	s, st := newService(t)
	ps := st.For(api.GlobalProject)
	root := t.TempDir() // never git-initialised — proves no git call is made
	wt := filepath.Join(root, "ori")
	a := store.Agent{Name: "ori", Role: "reviewer", Workspace: "ori"}

	if err := s.prepareWorkspace(ps, api.GlobalProject, "ori", root, wt, a); err != nil {
		t.Fatalf("prepareWorkspace: %v", err)
	}
	if fi, err := os.Stat(wt); err != nil || !fi.IsDir() {
		t.Fatalf("workspace directory not created: err=%v", err)
	}
}

// TestPrepareWorkspaceStillAddsAWorktreeForAnOrdinaryReviewer: the GlobalProject branch must not
// swallow the ordinary path — a project-bound reviewer still gets a real git worktree.
func TestPrepareWorkspaceStillAddsAWorktreeForAnOrdinaryReviewer(t *testing.T) {
	s, st := newService(t)
	root := newGitRepo(t)
	ps := st.For("repo")
	wt := filepath.Join(root, ".worktrees", "fili")
	a := store.Agent{Name: "fili", Role: "reviewer", Workspace: ".worktrees/fili"}

	if err := s.prepareWorkspace(ps, "repo", "fili", root, wt, a); err != nil {
		t.Fatalf("prepareWorkspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
		t.Errorf("expected a real git worktree at %s: %v", wt, err)
	}
}

// TestPrepareWorkspaceRefusesARepoWithNoCommits: unchanged behaviour for the ordinary path — a
// GlobalProject reviewer must not accidentally bypass this by some other route.
func TestPrepareWorkspaceRefusesARepoWithNoCommits(t *testing.T) {
	s, st := newService(t)
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git unavailable: %v: %s", err, out)
	}
	ps := st.For("repo")
	a := store.Agent{Name: "eitri", Role: "worker", Workspace: ".worktrees/eitri"}

	err := s.prepareWorkspace(ps, "repo", "eitri", root, filepath.Join(root, ".worktrees", "eitri"), a)
	if err == nil {
		t.Fatal("a repo with no commits should refuse to prepare a workspace")
	}
}

// noopAgentChanDeps is agentchan.Deps with nothing behind it — DeleteAgent's CloseAgent call only
// needs a real *agentchan.Server to not be nil, never these.
type noopAgentChanDeps struct{}

func (noopAgentChanDeps) Commands(string, string) (any, error) { return nil, nil }
func (noopAgentChanDeps) Directive(context.Context, string, string) (string, error) {
	return "", nil
}
func (noopAgentChanDeps) Exec(context.Context, string, string, []string, io.Writer) (int, error) {
	return 0, nil
}
func (noopAgentChanDeps) TokenAgent(string) (string, string, bool, error) { return "", "", false, nil }
func (noopAgentChanDeps) LogRequests(string, http.Handler) http.Handler   { return nil }

// rootedDeps is clearTestDeps with a real ProjectRoot, for a test that needs a place on disk to
// actually materialise a workspace under.
type rootedDeps struct {
	clearTestDeps
	root string
}

func (d rootedDeps) ProjectRoot(string) string { return d.root }

// TestDeleteAgentRemovesAPooledReviewersMaterialisedWorkspace is the mirror of
// TestPrepareWorkspaceForGlobalReviewerNeedsNoRepository: prepareWorkspace makes the fixed
// directory by MkdirAll, not WorktreeAdd, so DeleteAgent must remove it by RemoveAll, not
// WorktreeRemove — which fails silently against a path with no repository behind it and would
// otherwise leave the reviewed tree on disk forever.
func TestDeleteAgentRemovesAPooledReviewersMaterialisedWorkspace(t *testing.T) {
	_, st := newService(t)
	deps := rootedDeps{root: t.TempDir()}
	s := New(st, deps, agentchan.New(st, noopAgentChanDeps{}))
	if err := st.For(api.GlobalProject).PutAgent(store.Agent{Name: "ori", Role: "reviewer", Workspace: "ori"}); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(deps.root, "ori")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "src.go"), []byte("package x"), 0o644); err != nil {
		t.Fatal(err)
	}
	container.Use(&fakeRuntime{})
	t.Cleanup(container.UseDefault)

	if err := s.DeleteAgent(context.Background(), api.GlobalProject, "ori"); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("ori's materialised workspace should be gone, stat err = %v", err)
	}
}
