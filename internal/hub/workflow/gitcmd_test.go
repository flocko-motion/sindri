package workflow

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/adapter/git"
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
	if err := ps.SetState(store.AgentState{Agent: "eitri", Branch: "work", Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
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

// TestGitRollbackReturnsToAnEarlierPointOfItsOwn is the undo that replaced `git restore`: with the
// workspace recorded at every gate there are no unrecorded changes left to put back, so the unit an
// agent can undo is a step of its own history — and everything after it goes, edits included.
func TestGitRollbackReturnsToAnEarlierPointOfItsOwn(t *testing.T) {
	e, c, wt, _ := gitEngine(t)
	target, err := git.Head(wt)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(wt, "feature.go"), "package p\n\nfunc Feature() {}\n// a wrong turn\n")
	if _, err := e.gateCommit("proj", "eitri", "a wrong turn"); err != nil {
		t.Fatalf("gateCommit: %v", err)
	}
	writeFile(t, filepath.Join(wt, "scratch.go"), "package p // not handed over\n")

	out, code := gitVerb(t, e, c, "rollback", shortSHA(target))
	if code != 0 {
		t.Fatalf("`git rollback` failed: code=%d out=%q", code, out)
	}
	b, err := os.ReadFile(filepath.Join(wt, "feature.go"))
	if err != nil || strings.Contains(string(b), "wrong turn") {
		t.Fatalf("feature.go = %q (err %v), want the recorded wrong turn gone", b, err)
	}
	if !strings.Contains(string(b), "func Feature()") {
		t.Fatalf("rollback must keep everything up to its target, got %q", b)
	}
	if _, err := os.Stat(filepath.Join(wt, "scratch.go")); err == nil {
		t.Error("work never handed over must go too — a rollback that leaves it is not a rollback")
	}
	if head, _ := git.Head(wt); head != target {
		t.Errorf("HEAD = %q, want the named point %q", head, target)
	}
	if !strings.Contains(out, "back at") {
		t.Errorf("the reply should say where it left the agent, got %q", out)
	}
}

// TestGitRollbackRefusesWhatIsNotTheAgentsOwn: the verb takes an id, which is the one place an agent
// names something that is not a path — so it must reach only its own history. Neither a commit the
// reference branch already had, nor a made-up id, may move the branch.
func TestGitRollbackRefusesWhatIsNotTheAgentsOwn(t *testing.T) {
	e, c, wt, root := gitEngine(t)
	before, err := git.Head(wt)
	if err != nil {
		t.Fatal(err)
	}
	// The reference moves on, as it does under every agent: its new tip is a real commit in the repo
	// and no part of this branch, so naming it must be refused rather than checked out.
	writeFile(t, filepath.Join(root, "unrelated.go"), "package p // somebody else's work\n")
	if out, err := exec.Command("git", "-C", root, "commit", "-aqm", "moved on").CombinedOutput(); err != nil {
		t.Fatalf("advance main: %s", out)
	}
	moved, err := git.BranchTip(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, id, want string }{
		{"a made-up id", "0000000", "No 0000000"},
		{"not an id at all", "main", "not an id"},
		{"no id", "", "exactly one id"},
		{"somebody else's commit", shortSHA(moved), "isn't in your branch"},
	} {
		args := []string{"rollback"}
		if tc.id != "" {
			args = append(args, tc.id)
		}
		out, code := gitVerb(t, e, c, args...)
		if code == 0 {
			t.Errorf("%s: should be refused, got exit 0 and %q", tc.name, out)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: reply should say %q, got %q", tc.name, tc.want, out)
		}
	}
	if head, _ := git.Head(wt); head != before {
		t.Errorf("a refused rollback must move nothing: HEAD = %q, want %q", head, before)
	}
}

// TestGitRollbackWillNotRewindPastWhereTheBranchStarted: the reference's own history is reachable
// from the agent's HEAD, so "is it an ancestor" alone would let a rollback rewind into work the agent
// never did — leaving its branch DELETING files, and the reset out of reach of every verb it has.
func TestGitRollbackWillNotRewindPastWhereTheBranchStarted(t *testing.T) {
	e, c, wt, root := gitEngine(t)
	first, err := exec.Command("git", "-C", root, "rev-list", "--max-parents=0", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "unrelated.go"), "package p // a second commit on the reference\n")
	if out, e := exec.Command("git", "-C", root, "commit", "-aqm", "second").CombinedOutput(); e != nil {
		t.Fatalf("commit on the reference: %s", out)
	}
	// A branch that starts at the reference's SECOND commit: its first is then an ancestor of HEAD
	// and older than where this branch began — exactly the shape the guard exists for.
	second, err := git.BranchTip(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := git.ResetBranchTo(wt, second); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(wt, "mine.go"), "package p // my own work\n")
	if _, err := e.gateCommit("proj", "eitri", "my own work"); err != nil {
		t.Fatalf("gateCommit: %v", err)
	}
	head, err := git.Head(wt)
	if err != nil {
		t.Fatal(err)
	}

	out, code := gitVerb(t, e, c, "rollback", strings.TrimSpace(string(first))[:7])
	if code == 0 {
		t.Fatalf("rolling back into the reference's history must be refused, got %q", out)
	}
	if !strings.Contains(out, "not yours to undo") {
		t.Errorf("the refusal should say whose work that is, got %q", out)
	}
	if now, _ := git.Head(wt); now != head {
		t.Errorf("nothing may have moved: HEAD = %q, want %q", now, head)
	}
}

// TestTheDestructiveVerbsRefuseASharedCheckout: a coauthor's /workspace is the user's own tree. A
// rollback there is `reset --hard` plus `clean -fd` over whatever they have in progress, and a drop
// commits into it — so both are refused rather than performed on somebody else's behalf.
func TestTheDestructiveVerbsRefuseASharedCheckout(t *testing.T) {
	e, _, _, root := gitEngine(t)
	ps := e.store.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "loki", Role: "coauthor", Workspace: "."}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "the-users-work.txt"), "half-finished")
	c := registry.Caller{Project: "proj", Agent: "loki", Role: "coauthor"}
	head, err := git.Head(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{"rollback", shortSHA(head)}, {"drop", "feature.go"}} {
		out, code := gitVerb(t, e, c, args...)
		if code == 0 {
			t.Errorf("`git %s` on the user's checkout must be refused, got %q", args[0], out)
		}
		if !strings.Contains(out, "the user's own checkout") {
			t.Errorf("`git %s`: the refusal should say whose tree that is, got %q", args[0], out)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "the-users-work.txt")); err != nil {
		t.Error("the user's work in progress was destroyed")
	}
	if now, _ := git.Head(root); now != head {
		t.Errorf("the user's checkout moved: HEAD %q → %q", head, now)
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
		{"the withdrawn restore", []string{"restore"}, "is gone"},
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

// manualRepo returns a fresh repo root, the path its agent worktree will live at, and a `git -C`
// runner over either — the plumbing every hand-built fixture below needs before it can lay down
// whatever commit history and worktree state the case calls for; gitEngine's fixed shape can't
// express a repo that advances independently of the agent's branch or a path with no committed churn.
func manualRepo(t *testing.T) (root, wt string, run func(dir string, args ...string)) {
	t.Helper()
	root = t.TempDir()
	return root, filepath.Join(root, ".worktrees", "eitri"), func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git -C %s %v: %s", dir, args, out)
		}
	}
}

// manualEngine wires root/wt into a workflow Engine with one worker agent, "eitri", on branch
// "work" — called once the caller has already committed whatever history manualRepo's case needs.
func manualEngine(t *testing.T, root, wt string) (*Engine, registry.Caller) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("proj")
	rel, err := filepath.Rel(root, wt)
	if err != nil {
		t.Fatalf("worktree path: %v", err)
	}
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker", Workspace: rel}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Branch: "work", Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatalf("set state: %v", err)
	}
	return New(st, &stubDeps{root: root}), registry.Caller{Project: "proj", Agent: "eitri", Role: "worker"}
}

// TestGitDropSaysSoWhenNothingMoved is defect A: neither step gitDrop takes can fail on a no-op
// (checkout exits 0 when the paths already match; CommitAll is a no-op with nothing staged), so the
// unconditional success line was a claim about work that was never done.
func TestGitDropSaysSoWhenNothingMoved(t *testing.T) {
	root, wt, run := manualRepo(t)
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

	run(root, "worktree", "add", "-q", "-b", "work", wt)
	// Real work on a different file, so the branch is not empty — a drop on steady.go must leave it.
	if e := os.WriteFile(filepath.Join(wt, "feature.go"), []byte("package p\n\nfunc Feature() {}\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	run(wt, "add", "-A")
	run(wt, "commit", "-qm", "feature")

	e, c := manualEngine(t, root, wt)
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
	root, wt, run := manualRepo(t)
	run(root, "init", "-q", "-b", "main")
	run(root, "config", "user.email", "t@t")
	run(root, "config", "user.name", "t")
	if e := os.WriteFile(filepath.Join(root, "go.mod"), []byte("v1\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	run(root, "add", "-A")
	run(root, "commit", "-qm", "base")

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

	e, c := manualEngine(t, root, wt)

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

// TestGitDropCatchesCommittedChurnBehindAHandEditedWorktree is the review's blocking finding: a
// worktree-only no-op check can be fooled by hand-editing a path back to what it will end up
// matching, without committing — HEAD still carries the churn `git change` measures, but the
// worktree alone already looks settled. Mirrors TestGitDropAgreesWithChangeEvenAsTheReferenceAdvances
// with one extra step (the hand-edit) so it pins the exact shape the review described.
func TestGitDropCatchesCommittedChurnBehindAHandEditedWorktree(t *testing.T) {
	root, wt, run := manualRepo(t)
	run(root, "init", "-q", "-b", "main")
	run(root, "config", "user.email", "t@t")
	run(root, "config", "user.name", "t")
	if e := os.WriteFile(filepath.Join(root, "go.mod"), []byte("v1\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	run(root, "add", "-A")
	run(root, "commit", "-qm", "base")

	run(root, "worktree", "add", "-q", "-b", "work", wt)
	// The agent commits churn to go.mod: merge-base v1 -> committed v2.
	if e := os.WriteFile(filepath.Join(wt, "go.mod"), []byte("v2\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	run(wt, "add", "-A")
	run(wt, "commit", "-qm", "bump (churn)")
	// Then hand-edits it back to v1 in the worktree, uncommitted — "tidying up" before asking drop
	// to be sure. HEAD still holds v2; only the worktree looks like the merge-base.
	if e := os.WriteFile(filepath.Join(wt, "go.mod"), []byte("v1\n"), 0o644); e != nil {
		t.Fatal(e)
	}

	e, c := manualEngine(t, root, wt)

	out, code := gitVerb(t, e, c, "drop", "go.mod")
	if code != 0 {
		t.Fatalf("`git drop` failed: code=%d out=%q", code, out)
	}
	if strings.Contains(out, "already match") || strings.Contains(out, "nothing recorded") {
		t.Fatalf("HEAD still carries the committed churn — a hand-edited worktree must not read as a no-op: %q", out)
	}
	if !strings.Contains(out, "Removed") {
		t.Errorf("the committed churn should be dropped and recorded, got %q", out)
	}

	out, code = gitVerb(t, e, c, "change", "go.mod")
	if code != 0 {
		t.Fatalf("`git change` after drop: code=%d out=%q", code, out)
	}
	if !strings.HasPrefix(out, "No ") {
		t.Errorf("`git drop` claimed to have recorded a fix but `git change` still shows go.mod: %q", out)
	}
}

// TestGitDropClearsADirtyEditWithNothingToRecord is the review's milder finding, other direction:
// HEAD already agrees with the merge-base (no committed churn), but the worktree carries an
// uncommitted edit — say, from a drop run a second time after a further hand-edit. Something real
// happens (the edit is cleared), but no commit results, and the reply must not claim one.
func TestGitDropClearsADirtyEditWithNothingToRecord(t *testing.T) {
	root, wt, run := manualRepo(t)
	run(root, "init", "-q", "-b", "main")
	run(root, "config", "user.email", "t@t")
	run(root, "config", "user.name", "t")
	if e := os.WriteFile(filepath.Join(root, "go.mod"), []byte("v1\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	run(root, "add", "-A")
	run(root, "commit", "-qm", "base")

	run(root, "worktree", "add", "-q", "-b", "work", wt)
	// The branch never actually diverges from the merge-base in its commits...
	if e := os.WriteFile(filepath.Join(wt, "feature.go"), []byte("package p\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	run(wt, "add", "-A")
	run(wt, "commit", "-qm", "unrelated feature commit")
	// ...but go.mod sits dirty in the worktree, uncommitted.
	if e := os.WriteFile(filepath.Join(wt, "go.mod"), []byte("scratch edit\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	before := gitLog(t, wt)

	e, c := manualEngine(t, root, wt)

	out, code := gitVerb(t, e, c, "drop", "go.mod")
	if code != 0 {
		t.Fatalf("`git drop` failed: code=%d out=%q", code, out)
	}
	if strings.Contains(out, "Removed") {
		t.Errorf("no commit resulted (HEAD already matched) — the reply must not claim one: %q", out)
	}
	if b, err := os.ReadFile(filepath.Join(wt, "go.mod")); err != nil || string(b) != "v1\n" {
		t.Fatalf("go.mod = %q (err %v), want the dirty edit cleared back to HEAD's content", b, err)
	}
	if got := gitLog(t, wt); got != before {
		t.Errorf("nothing should have been committed — log was %q, now %q", before, got)
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
