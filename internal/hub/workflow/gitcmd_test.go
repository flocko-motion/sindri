package workflow

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// gitEngine mirrors the real layout: the main checkout stays on the reference branch (which is what
// baseBranch reads) and the agent works in a linked worktree on its own branch. It returns the
// engine, the caller, the agent's worktree and the main checkout's root.
func gitEngine(t *testing.T) (*Engine, registry.Caller, string, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	runIn := func(dir string) func(...string) {
		return func(args ...string) {
			t.Helper()
			if out, e := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); e != nil {
				t.Fatalf("git -C %s %v: %s", dir, args, out)
			}
		}
	}
	run := runIn(root)
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if e := os.WriteFile(filepath.Join(root, "feature.go"), []byte("package p\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, "unrelated.go"), []byte("package p // original\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	run("add", "-A")
	run("commit", "-qm", "base")

	// The agent's own worktree on its own branch; the root stays on main throughout.
	wt := filepath.Join(root, ".worktrees", "eitri")
	run("worktree", "add", "-q", "-b", "work", wt)
	runWT := runIn(wt)
	if e := os.WriteFile(filepath.Join(wt, "feature.go"), []byte("package p\n\nfunc Feature() {}\n"), 0o644); e != nil {
		t.Fatal(e) // the real work
	}
	if e := os.WriteFile(filepath.Join(wt, "unrelated.go"), []byte("package p // churn nobody asked for\n"), 0o644); e != nil {
		t.Fatal(e) // committed churn
	}
	runWT("add", "-A")
	runWT("commit", "-qm", "feature plus churn")

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
	return New(st, &stubDeps{root: root}), registry.Caller{Project: "proj", Agent: "eitri", Role: "worker"}, wt, root
}

// exec runs the git verb and returns its output plus exit code.
func gitVerb(t *testing.T, e *Engine, c registry.Caller, args ...string) (string, int) {
	t.Helper()
	var b strings.Builder
	code, err := e.CmdGit(c, args, &b)
	if err != nil {
		t.Fatalf("CmdGit %v: %v", args, err)
	}
	return b.String(), code
}

// gitActions pulls the action names out of a listing: only lines that START the entry ("git <name>"
// in GitHelp, "sindri git <name>" in the shim), so prose mentioning git is not read as an action.
func gitActions(t *testing.T, listing string) map[string]bool {
	t.Helper()
	found := map[string]bool{}
	for _, line := range strings.Split(listing, "\n") {
		f := strings.Fields(line)
		if len(f) > 2 && f[0] == "sindri" && f[1] == "git" {
			found[f[2]] = true
			continue
		}
		if len(f) > 1 && f[0] == "git" {
			found[f[1]] = true
		}
	}
	return found
}

// TestPodGitShimListsTheSameActions guards a duplication that cannot be removed: the pod's git shim
// is a shell script embedded in the image, so it repeats the action list rather than reading GitHelp.
// A stale copy would misdirect an agent at the exact moment it is already stuck — the failure this
// whole surface exists to prevent — so drift fails here instead of in a pod.
func TestPodGitShimListsTheSameActions(t *testing.T) {
	shim, err := os.ReadFile(filepath.Join("..", "..", "container", "buildctx", "shims", "git"))
	if err != nil {
		t.Fatalf("read the pod git shim: %v", err)
	}
	want, got := gitActions(t, GitHelp), gitActions(t, string(shim))
	// Both empty would make the comparison below pass while checking nothing.
	if len(want) < 2 {
		t.Fatalf("parsed %d actions out of GitHelp — the extractor is broken, not the shim", len(want))
	}
	for a := range want {
		if !got[a] {
			t.Errorf("the pod git shim doesn't mention `sindri git %s` — an agent reading it is misdirected", a)
		}
	}
	for a := range got {
		if !want[a] {
			t.Errorf("the pod git shim offers `sindri git %s`, which is not a real action", a)
		}
	}
}

// TestGitDropRemovesCommittedChurn is the case an agent could not carry out at all: told to drop its
// changes to unrelated files, with no git in the pod, it hand-wrote a script that guessed at the
// original content. `git drop` restores those paths from the reference branch and commits, so the
// files leave the change while the real work stays.
func TestGitDropRemovesCommittedChurn(t *testing.T) {
	e, c, wt, _ := gitEngine(t)

	out, code := gitVerb(t, e, c, "change")
	if code != 0 || !strings.Contains(out, "unrelated.go") || !strings.Contains(out, "feature.go") {
		t.Fatalf("`git change` must show the whole change, got code=%d out=%q", code, out)
	}
	// The real branch name must never appear in what an agent is told.
	if strings.Contains(out, "main") {
		t.Errorf("reply names the reference branch: %q", out)
	}

	out, code = gitVerb(t, e, c, "drop", "unrelated.go")
	if code != 0 {
		t.Fatalf("`git drop` failed: code=%d out=%q", code, out)
	}
	if b, err := os.ReadFile(filepath.Join(wt, "unrelated.go")); err != nil || string(b) != "package p // original\n" {
		t.Fatalf("unrelated.go = %q (err %v), want the reference branch's content", b, err)
	}
	// The work survives, and the drop is committed so it actually leaves the change.
	if b, err := os.ReadFile(filepath.Join(wt, "feature.go")); err != nil || !strings.Contains(string(b), "func Feature()") {
		t.Fatalf("feature.go = %q (err %v), want the work untouched", b, err)
	}
	out, _ = gitVerb(t, e, c, "change")
	if strings.Contains(out, "unrelated.go") {
		t.Errorf("unrelated.go is still in the change after being dropped: %q", out)
	}
	if !strings.Contains(out, "feature.go") {
		t.Errorf("the real work should remain in the change: %q", out)
	}
}

// TestGitRestoreDiscardsOnlyUncommitted separates the two destructive actions: restore puts back
// what is uncommitted, and leaves committed work alone (that is what drop is for).
func TestGitRestoreDiscardsOnlyUncommitted(t *testing.T) {
	e, c, wt, _ := gitEngine(t)
	if err := os.WriteFile(filepath.Join(wt, "feature.go"), []byte("package p\n\nfunc Feature() {}\n// scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := gitVerb(t, e, c, "status")
	if code != 0 || !strings.Contains(out, "feature.go") {
		t.Fatalf("`git status` must list the uncommitted change, got code=%d out=%q", code, out)
	}
	if out, code = gitVerb(t, e, c, "restore", "feature.go"); code != 0 {
		t.Fatalf("`git restore` failed: code=%d out=%q", code, out)
	}
	b, err := os.ReadFile(filepath.Join(wt, "feature.go"))
	if err != nil || strings.Contains(string(b), "scratch") {
		t.Fatalf("feature.go = %q (err %v), want the scratch edit gone", b, err)
	}
	if !strings.Contains(string(b), "func Feature()") {
		t.Fatalf("restore must not touch committed work, got %q", b)
	}
	out, _ = gitVerb(t, e, c, "status")
	if !strings.Contains(out, "Nothing to show") {
		t.Errorf("status should be clean after the restore, got %q", out)
	}
}

// TestGitRefusesWhatIsNotAllowed: the hub is the gatekeeper, so an unlisted action, a flag, a path
// escaping the workspace and an unscoped destructive call are all refused rather than passed to git.
func TestGitRefusesWhatIsNotAllowed(t *testing.T) {
	e, c, _, _ := gitEngine(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"unlisted action", []string{"push"}, "not available"},
		{"raw flag", []string{"diff", "--stat"}, "looks like a flag"},
		{"absolute path", []string{"diff", "/etc/passwd"}, "absolute path"},
		{"escaping path", []string{"diff", "../../etc/passwd"}, "outside your workspace"},
		{"unscoped restore", []string{"restore"}, "needs the paths"},
		{"unscoped drop", []string{"drop"}, "needs the paths"},
	} {
		out, code := gitVerb(t, e, c, tc.args...)
		if code == 0 {
			t.Errorf("%s: should be refused, got exit 0 and %q", tc.name, out)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: reply should say %q, got %q", tc.name, tc.want, out)
		}
	}
}

// TestGitIncomingShowsWhatMoved: an agent has no way to see what the reference branch gained, so it
// guesses at what changed under it. `incoming` answers that, and names no branch while doing it.
func TestGitIncomingShowsWhatMoved(t *testing.T) {
	e, c, _, root := gitEngine(t)
	out, _ := gitVerb(t, e, c, "incoming")
	if !strings.Contains(out, "Nothing new") {
		t.Fatalf("nothing has moved yet, got %q", out)
	}
	// The reference branch gains a commit while the agent works — committed in the main checkout,
	// which is already on it; the agent's worktree stays put, as the real drift happens.
	if err := os.WriteFile(filepath.Join(root, "feature.go"), []byte("package p // upstream moved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", root, "commit", "-aqm", "upstream change").CombinedOutput(); err != nil {
		t.Fatalf("commit upstream: %s", out)
	}

	out, code := gitVerb(t, e, c, "incoming")
	if code != 0 || !strings.Contains(out, "upstream change") {
		t.Fatalf("`git incoming` must name the new commit, got code=%d out=%q", code, out)
	}
	if strings.Contains(out, "main") {
		t.Errorf("reply names the reference branch: %q", out)
	}
	// And the agent's own recorded work is a separate question with a separate answer.
	out, _ = gitVerb(t, e, c, "history")
	if !strings.Contains(out, "feature plus churn") {
		t.Errorf("`git history` should list the agent's own recorded work, got %q", out)
	}
}

// TestGitDropSaysSoWhenNothingMoved is defect A: neither step gitDrop takes can fail on a no-op
// (checkout exits 0 when the paths already match; CommitAll is a no-op with nothing staged), so the
// unconditional success line was a claim about work that was never done.
func TestGitDropSaysSoWhenNothingMoved(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
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
	// steady.go is never touched by anyone — the one path that already matches wherever a branch and
	// the reference last agreed, which is exactly what a no-op drop should find.
	if e := os.WriteFile(filepath.Join(root, "steady.go"), []byte("package p // never touched\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	run(root, "add", "-A")
	run(root, "commit", "-qm", "base")

	wt := filepath.Join(root, ".worktrees", "eitri")
	run(root, "worktree", "add", "-q", "-b", "work", wt)
	// Real work on a different file, so the branch is not empty — a drop on steady.go must leave it.
	if e := os.WriteFile(filepath.Join(wt, "feature.go"), []byte("package p\n\nfunc Feature() {}\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	run(wt, "add", "-A")
	run(wt, "commit", "-qm", "feature")

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
	e := New(st, &stubDeps{root: root})
	c := registry.Caller{Project: "proj", Agent: "eitri", Role: "worker"}
	before := gitLog(t, wt)

	out, code := gitVerb(t, e, c, "drop", "steady.go")
	if code != 0 {
		t.Fatalf("`git drop` on an already-matching path should still succeed, got code=%d out=%q", code, out)
	}
	for _, want := range []string{"already match", "nothing to drop", "nothing recorded"} {
		if !strings.Contains(out, want) {
			t.Errorf("a no-op drop should say so plainly, got %q (missing %q)", out, want)
		}
	}
	if strings.Contains(out, "Removed") {
		t.Errorf("a no-op must not claim to have removed anything: %q", out)
	}
	if got := gitLog(t, wt); got != before {
		t.Errorf("a no-op drop must not record a commit — log was %q, now %q", before, got)
	}
}

// gitLog is the worktree's current commit history, short-form — used to assert that a no-op drop
// records nothing, since HasChanges alone cannot tell "nothing to commit" from "never tried".
func gitLog(t *testing.T, wt string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", wt, "log", "--format=%h").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %s", out)
	}
	return string(out)
}

// TestGitDropAgreesWithChangeEvenAsTheReferenceAdvances is defect B, reproducing the exact report:
// the agent's branch bumps a file to content the reference branch LATER bumps to as well,
// independently. Dropping against the reference's current TIP would then be a no-op (the file
// already matches there) while `change`'s three-dot diff — merge-base, not tip — still shows it.
// Dropping against the merge-base instead keeps the two answers talking about the same commit.
func TestGitDropAgreesWithChangeEvenAsTheReferenceAdvances(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
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
	if e := os.WriteFile(filepath.Join(root, "go.mod"), []byte("v1\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	run(root, "add", "-A")
	run(root, "commit", "-qm", "base")

	wt := filepath.Join(root, ".worktrees", "eitri")
	run(root, "worktree", "add", "-q", "-b", "work", wt)

	// The agent bumps go.mod on its own branch, unrelated to its actual task.
	if e := os.WriteFile(filepath.Join(wt, "go.mod"), []byte("v2\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	run(wt, "add", "-A")
	run(wt, "commit", "-qm", "bump (churn)")

	// The reference branch ALSO advances to v2, independently, after the agent forked — the case
	// that made the first bug report's checkout a no-op.
	if e := os.WriteFile(filepath.Join(root, "go.mod"), []byte("v2\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	run(root, "commit", "-aqm", "upstream also bumped")

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
	e := New(st, &stubDeps{root: root})
	c := registry.Caller{Project: "proj", Agent: "eitri", Role: "worker"}

	out, code := gitVerb(t, e, c, "change", "go.mod")
	if code != 0 || !strings.Contains(out, "go.mod") {
		t.Fatalf("`git change` should show go.mod diverging, got code=%d out=%q", code, out)
	}

	out, code = gitVerb(t, e, c, "drop", "go.mod")
	if code != 0 {
		t.Fatalf("`git drop` failed: code=%d out=%q", code, out)
	}
	if strings.Contains(out, "already match") {
		t.Fatalf("dropping to the merge-base is a real change here (v2 -> v1) — must not read as a no-op: %q", out)
	}
	if b, err := os.ReadFile(filepath.Join(wt, "go.mod")); err != nil || string(b) != "v1\n" {
		t.Fatalf("go.mod = %q (err %v), want the merge-base's content (v1), not the reference's current tip (v2)", b, err)
	}

	out, code = gitVerb(t, e, c, "change", "go.mod")
	if code != 0 {
		t.Fatalf("`git change` after drop: code=%d out=%q", code, out)
	}
	// "No whole change ... in go.mod" (scopeNote) is the answer wanted; anything with a diff hunk in
	// it means change still measured the file as part of the branch's introduced content.
	if !strings.HasPrefix(out, "No ") {
		t.Errorf("`git drop` reported success but `git change` still shows go.mod — the two disagreed: %q", out)
	}
}

// TestBaseBranchWarnsOnceAboutAnUnconfiguredReference is defect C: with no `reference:` set, every
// agent measures against whatever the human happens to have checked out in the main working copy,
// silently. That deserves at least a warning, logged once per repo rather than on every call —
// baseBranch runs on nearly every git/PR verb, and a line per call would drown out everything else.
func TestBaseBranchWarnsOnceAboutAnUnconfiguredReference(t *testing.T) {
	e, _, _, root := gitEngine(t)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w

	if _, err := e.baseBranch(root); err != nil {
		t.Fatalf("baseBranch: %v", err)
	}
	if _, err := e.baseBranch(root); err != nil {
		t.Fatalf("baseBranch (second call): %v", err)
	}
	w.Close()
	os.Stderr = orig
	out, _ := io.ReadAll(r)

	if got := strings.Count(string(out), "no `reference:` configured"); got != 1 {
		t.Errorf("warned %d time(s) across two calls, want exactly 1 (deduped by repo root): %q", got, out)
	}
	if !strings.Contains(string(out), root) {
		t.Errorf("the warning should name which repo it's about, got %q", out)
	}
}
