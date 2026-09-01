package workflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// scratchEngine is the real layout a coauthor lives in: a repo with a worker's branch and PR, the
// coauthor's /workspace being the root itself, and its scratch worktree beside the worker's. It
// returns the engine, the coauthor caller and the scratch tree's host path.
func scratchEngine(t *testing.T) (*Engine, registry.Caller, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		if out, e := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); e != nil {
			t.Fatalf("git -C %s %v: %s", dir, args, out)
		}
	}
	run(root, "init", "-q", "-b", "main")
	run(root, "config", "user.email", "t@t")
	run(root, "config", "user.name", "t")
	if e := os.WriteFile(filepath.Join(root, "base.go"), []byte("package p\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	run(root, "add", "-A")
	run(root, "commit", "-qm", "base")

	// The worker's branch, with the change a coauthor would want to test.
	wt := filepath.Join(root, ".worktrees", "dvalin")
	run(root, "worktree", "add", "-q", "-b", "sd-1", wt)
	if e := os.WriteFile(filepath.Join(wt, "feature.go"), []byte("package p\n\nfunc Feature() {}\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	run(wt, "add", "-A")
	run(wt, "commit", "-qm", "the feature")

	// The coauthor: /workspace is the root, and its scratch tree is what the launcher adds.
	scratch := filepath.Join(root, ScratchWorktree("brokk"))
	run(root, "worktree", "add", "-q", "--detach", scratch, "main")

	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	for _, a := range []store.Agent{
		{Name: "brokk", Role: "coauthor", Workspace: "."},
		{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"},
	} {
		if err := ps.PutAgent(a); err != nil {
			t.Fatal(err)
		}
	}
	if err := ps.PutPR(store.PR{ID: "pr-sd-1", Task: "sd-1", Agent: "dvalin", Branch: "sd-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	return newEngine(st, &stubDeps{root: root}), registry.Caller{Project: "proj", Agent: "brokk", Role: "coauthor"}, scratch
}

// scratchVerb runs the verb and returns its output and exit code.
func scratchVerb(t *testing.T, e *Engine, c registry.Caller, args ...string) (string, int) {
	t.Helper()
	var b strings.Builder
	code, err := e.CmdScratch(c, args, &b)
	if err != nil {
		t.Fatalf("CmdScratch %v: %v", args, err)
	}
	return b.String(), code
}

// TestScratchTakesAPRIdAsWellAsARef: "let me look at pr-sd-1" is the common case, so the hub resolves
// it to the branch rather than making the agent look it up — and the checkout is detached, because
// the author's worktree holds that branch and git gives a branch to one worktree at a time.
func TestScratchTakesAPRIdAsWellAsARef(t *testing.T) {
	e, c, scratch := scratchEngine(t)

	out, code := scratchVerb(t, e, c, "pr-sd-1")
	if code != 0 {
		t.Fatalf("scratch pr-sd-1: code=%d out=%s", code, out)
	}
	if !strings.Contains(out, ScratchMount) || !strings.Contains(out, "sd-1") {
		t.Errorf("the reply should name what landed where, got:\n%s", out)
	}
	// The PR's work is there, which is the whole point: it can build and test it now.
	if _, err := os.Stat(filepath.Join(scratch, "feature.go")); err != nil {
		t.Errorf("the PR's change is not in the scratch tree: %v", err)
	}
	if branch, err := exec.Command("git", "-C", scratch, "symbolic-ref", "-q", "HEAD").Output(); err == nil {
		t.Errorf("the scratch tree is on branch %q — it must be detached, since its author holds it",
			strings.TrimSpace(string(branch)))
	}
	// A plain ref works the same way, so nothing about the PR path is a second mechanism.
	if out, code := scratchVerb(t, e, c, "main"); code != 0 {
		t.Fatalf("scratch main: code=%d out=%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(scratch, "feature.go")); err == nil {
		t.Error("checking main out left the previous checkout's files behind")
	}
}

// TestADirtyScratchRefusesRatherThanLosingWork: a scratch tree is exactly where a half-finished
// experiment lives, and nothing in it is recorded anywhere — so a checkout over it is the one and
// only warning the agent gets.
func TestADirtyScratchRefusesRatherThanLosingWork(t *testing.T) {
	e, c, scratch := scratchEngine(t)
	experiment := filepath.Join(scratch, "experiment.go")
	if err := os.WriteFile(experiment, []byte("package p // half an idea\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := scratchVerb(t, e, c, "pr-sd-1")
	if code == 0 {
		t.Fatalf("a dirty scratch must refuse, got:\n%s", out)
	}
	if _, err := os.Stat(experiment); err != nil {
		t.Errorf("the refused checkout discarded the work anyway: %v", err)
	}
	// The refusal has to name the way out, or it is a dead end.
	for _, want := range []string{"--force", "/workspace"} {
		if !strings.Contains(out, want) {
			t.Errorf("the refusal should name %q, got:\n%s", want, out)
		}
	}
	// And --force means it: the experiment goes, the PR's work arrives.
	if out, code := scratchVerb(t, e, c, "pr-sd-1", "--force"); code != 0 {
		t.Fatalf("scratch --force: code=%d out=%s", code, out)
	}
	if _, err := os.Stat(experiment); err == nil {
		t.Error("--force left the discarded experiment in place")
	}
	if _, err := os.Stat(filepath.Join(scratch, "feature.go")); err != nil {
		t.Errorf("--force did not check the PR out: %v", err)
	}
}

// TestScratchWithNoRefSaysWhatItTakes: the verb's own usage is an agent's only way to learn the
// surface, so an empty call answers with it rather than doing something.
func TestScratchWithNoRefSaysWhatItTakes(t *testing.T) {
	e, c, _ := scratchEngine(t)
	out, code := scratchVerb(t, e, c)
	if code != 2 {
		t.Errorf("code = %d, want 2 (usage)", code)
	}
	for _, want := range []string{"pr-id", "--force", ScratchMount} {
		if !strings.Contains(out, want) {
			t.Errorf("the usage should mention %q, got:\n%s", want, out)
		}
	}
}

// TestScratchSaysSoWhenThereIsNoTree: a coauthor whose pod started before it had a scratch tree gets
// the real reason and the fix, rather than git's words about a directory it never heard of.
func TestScratchSaysSoWhenThereIsNoTree(t *testing.T) {
	e, c, scratch := scratchEngine(t)
	if out, err := exec.Command("git", "-C", filepath.Dir(filepath.Dir(scratch)), "worktree", "remove", "--force", scratch).CombinedOutput(); err != nil {
		t.Fatalf("remove scratch worktree: %s", out)
	}
	out, code := scratchVerb(t, e, c, "main")
	if code == 0 {
		t.Fatalf("with no scratch tree there is nothing to check out into, got:\n%s", out)
	}
	if !strings.Contains(out, "relaunch") {
		t.Errorf("the reply should say how to get one, got:\n%s", out)
	}
}
