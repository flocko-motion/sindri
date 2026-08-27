package workflow

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// plannerIn builds a planner resting in phase and returns its engine, caller and store.
func plannerIn(t *testing.T, phase string) (*Engine, registry.Caller, *store.ProjectStore) {
	t.Helper()
	e, c, ps := plannerEngine(t, "td-1", "")
	if err := ps.PutAgent(store.Agent{Name: c.Agent, Role: "planner"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: c.Agent, Phase: phase}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	return e, c, ps
}

// TestPlannerKickoffCarriesItsDirective is the round trip the observation counted: a planner woken
// with MsgKickoff ran `sindri` to be told to carry on with the user, roughly eight times in one
// session. The hub knows the role, so the answer travels with the wake-up.
func TestPlannerKickoffCarriesItsDirective(t *testing.T) {
	for phase, want := range map[string]string{"": DirPlanner, "idle": DirPlanner, "planning": DirPlanning} {
		e, c, _ := plannerIn(t, phase)
		k := e.Kickoff(c.Project, c.Agent)
		if k == MsgKickoff {
			t.Errorf("phase %q: a planner was sent to fetch an answer that never varies: %q", phase, k)
		}
		if !strings.Contains(k, want) {
			t.Errorf("phase %q: the kickoff should carry the planner's own directive: %q", phase, k)
		}
		if strings.Contains(k, "Run `sindri`") {
			t.Errorf("phase %q: with nothing waiting, the kickoff must not ask for a call: %q", phase, k)
		}
	}
}

// TestWorkerAndReviewerKickoffsStillFetch: the hub holds their next job, so the fetch buys something.
func TestWorkerAndReviewerKickoffsStillFetch(t *testing.T) {
	for _, role := range []string{"worker", "reviewer", "coauthor"} {
		e, c, ps := plannerIn(t, "idle")
		if err := ps.PutAgent(store.Agent{Name: c.Agent, Role: role}); err != nil {
			t.Fatal(err)
		}
		if k := e.Kickoff(c.Project, c.Agent); k != MsgKickoff {
			t.Errorf("a %s should still be sent to `sindri` for its job, got %q", role, k)
		}
	}
}

// TestEscalatedPlannerKickoffServesTheEscalation: a planner waking mid-escalation must hear about
// that rather than the standing "carry on with the user" its phase would otherwise produce.
func TestEscalatedPlannerKickoffServesTheEscalation(t *testing.T) {
	e, c, ps := plannerIn(t, "planning")
	const q = "sd-3a4870 has an empty description — delete it, or tell me the work behind it?"
	if err := ps.SetEscalation(c.Agent, q); err != nil {
		t.Fatal(err)
	}
	k := e.Kickoff(c.Project, c.Agent)
	if !strings.Contains(k, q) || !strings.Contains(k, "ESCALATED") {
		t.Errorf("the kickoff should repeat what it is waiting on: %q", k)
	}
	if strings.Contains(k, DirPlanning) {
		t.Errorf("the escalation must outrank the role text, not be appended to it: %q", k)
	}
}

// TestPlannerKickoffNamesUnreadMail is the one thing a call to `sindri` does buy at that moment, so
// the kickoff asks for it — the exception that keeps "don't instruct a pointless fetch" honest.
func TestPlannerKickoffNamesUnreadMail(t *testing.T) {
	e, c, ps := plannerIn(t, "idle")
	if _, err := ps.AddMail(c.Agent, "hub", "td-9 was approved", false, 0); err != nil {
		t.Fatal(err)
	}
	k := e.Kickoff(c.Project, c.Agent)
	for _, want := range []string{"1 unread message(s)", "Run `sindri`", DirPlanner} {
		if !strings.Contains(k, want) {
			t.Errorf("the kickoff should name %q: %q", want, k)
		}
	}
}

// divergedRepo builds root/workspace as a repo whose "feat" branch and "main" have both moved, and
// returns its path plus writers for committing and for leaving a file dirty. Real git state rather
// than a planted marker file, since RebaseInProgress asks git where its own state lives.
func divergedRepo(t *testing.T, root, workspace, branchFile string) (dir string, write func(string, string)) {
	t.Helper()
	dir = filepath.Join(root, workspace)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	write = func(rel, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	write("f", "base\n")
	write("other", "base\n")
	run("add", "-A")
	run("commit", "-qm", "init")
	// branchFile decides whether the rebase itself conflicts: "f" clashes with main's move below,
	// "other" applies cleanly and leaves the autostash as the only thing that can clash.
	run("checkout", "-q", "-b", "feat")
	write(branchFile, "feat\n")
	run("add", "-A")
	run("commit", "-qm", "feat")
	run("checkout", "-q", "main")
	write("f", "main\n")
	run("add", "-A")
	run("commit", "-qm", "main moves f")
	run("checkout", "-q", "feat")
	return dir, write
}

// stoppedRebase leaves root/workspace holding a real rebase halted on a conflict.
func stoppedRebase(t *testing.T, root, workspace string) {
	t.Helper()
	dir, _ := divergedRepo(t, root, workspace, "f")
	if _, done, err := git.RebaseStart(dir, "feat", "main"); err != nil || done {
		t.Fatalf("expected the rebase to stop on a conflict, got done=%v err=%v", done, err)
	}
	if !git.RebaseInProgress(dir) {
		t.Fatal("fixture failed to leave a rebase in progress")
	}
}

// strandedAutostash leaves root/workspace with the rebase COMPLETE and its autostash unmerged — the
// state that reports "Successfully rebased" while refusing every checkout afterwards.
func strandedAutostash(t *testing.T, root, workspace string) {
	t.Helper()
	dir, write := divergedRepo(t, root, workspace, "other")
	write("f", "loose\n") // uncommitted and clashing: the autostash will fail to re-apply
	if _, _, err := git.RebaseStart(dir, "feat", "main"); err != nil {
		t.Fatalf("RebaseStart: %v", err)
	}
	if git.RebaseInProgress(dir) || !git.StashConflict(dir) {
		t.Fatalf("fixture wanted a stranded autostash, got inProgress=%v stash=%v",
			git.RebaseInProgress(dir), git.StashConflict(dir))
	}
}

// TestMidRebaseFrontsAPlannersDirective is the state a planner sat in: 49 conflicted paths and a
// directive reading "the hub is not waiting on a command from you and has nothing to add".
func TestMidRebaseFrontsAPlannersDirective(t *testing.T) {
	e, c, ps := plannerIn(t, "planning")
	root := e.deps.ProjectRoot(c.Project)
	stoppedRebase(t, root, "wt")
	if err := ps.PutAgent(store.Agent{Name: c.Agent, Role: "planner", Workspace: "wt"}); err != nil {
		t.Fatal(err)
	}
	dir, err := e.AgentDirective(context.Background(), c.Project, c.Agent)
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	for _, want := range []string{"UNRESOLVED CONFLICTS", "sindri rebase", DirPlanning} {
		if !strings.Contains(dir, want) {
			t.Errorf("the directive should name %q: %q", want, dir)
		}
	}
	if strings.Index(dir, "UNRESOLVED CONFLICTS") > strings.Index(dir, DirPlanning) {
		t.Errorf("the branch warning belongs ahead of the role text: %q", dir)
	}
	// The wake-up carries the same warning, since it now carries the directive itself.
	if !strings.Contains(e.Kickoff(c.Project, c.Agent), "UNRESOLVED CONFLICTS") {
		t.Errorf("a planner woken onto a stuck branch should be told: %q", e.Kickoff(c.Project, c.Agent))
	}
}

// TestMidRebaseNamesResolveForASubmittedBranch: a branch halted while resolving for its own PR is
// continued by `resolve`, which renews the PR — `rebase` would leave it out of review.
func TestMidRebaseNamesResolveForASubmittedBranch(t *testing.T) {
	e, c, ps := plannerIn(t, "resolving")
	stoppedRebase(t, e.deps.ProjectRoot(c.Project), "wt")
	if err := ps.PutAgent(store.Agent{Name: c.Agent, Role: "worker", Workspace: "wt"}); err != nil {
		t.Fatal(err)
	}
	notice := e.rebaseNotice(c.Project, c.Agent)
	if !strings.Contains(notice, "sindri resolve") {
		t.Errorf("a resolving branch should be pointed at resolve: %q", notice)
	}
}

// TestStrandedAutostashIsToldToo is the state CmdResolve pairs with a halted rebase: the commits
// landed, git said "Successfully rebased", and only the autostash clashed — so RebaseInProgress
// alone reads the branch as fine while every later checkout is refused over the unmerged index.
func TestStrandedAutostashIsToldToo(t *testing.T) {
	e, c, ps := plannerIn(t, "planning")
	strandedAutostash(t, e.deps.ProjectRoot(c.Project), "wt")
	if err := ps.PutAgent(store.Agent{Name: c.Agent, Role: "planner", Workspace: "wt"}); err != nil {
		t.Fatal(err)
	}
	if n := e.rebaseNotice(c.Project, c.Agent); !strings.Contains(n, "UNRESOLVED CONFLICTS") {
		t.Errorf("a stranded autostash leaves the branch equally unsubmittable: %q", n)
	}
}

// TestNoNoticeOnACleanBranch: the warning is a fact about the worktree, so it stays silent otherwise.
func TestNoNoticeOnACleanBranch(t *testing.T) {
	e, c, ps := plannerIn(t, "planning")
	if err := ps.PutAgent(store.Agent{Name: c.Agent, Role: "planner", Workspace: "wt"}); err != nil {
		t.Fatal(err)
	}
	if n := e.rebaseNotice(c.Project, c.Agent); n != "" {
		t.Errorf("no rebase is in progress, so nothing should be said: %q", n)
	}
}
