package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// lintVerb runs the `lint` verb as an agent does, returning its reply and exit code.
func lintVerb(t *testing.T, e *Engine, agent string, args ...string) (string, int) {
	t.Helper()
	var b strings.Builder
	code, err := e.CmdLint(registry.Caller{Project: "repo", Agent: agent, Role: "worker"}, args, &b)
	if err != nil {
		t.Fatalf("CmdLint %v: %v", args, err)
	}
	return b.String(), code
}

// writeExec writes an executable script, for a fixture that declares its own gate.
func writeExec(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestASelfCheckMeasuresTheCommitNotTheMovingTree is the invariant this whole change is named for. A
// self-check parks nobody — the hub tells that agent to carry on with something else while it waits —
// so gating its live worktree would build and test a moving target for minutes, report violations
// about a half-written file the agent never asked about, and file the result under a sha it was never
// taken on. An edit undone before the next gate would then be reused as a pass about that commit.
func TestASelfCheckMeasuresTheCommitNotTheMovingTree(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	e.deps.(*stubDeps).projectConfig = config.Config{Verify: "./check.sh"}
	wt := filepath.Join(root, ".worktrees", "bombur")
	// The project's own gate refuses if the tree carries scratch.txt — so the check's verdict says
	// which tree it read, rather than a test having to trust that it read the right one.
	writeExec(t, filepath.Join(wt, "check.sh"), "#!/bin/sh\ntest ! -f scratch.txt\n")

	r := openGate(t, e, "bombur", gateLint, "")
	writeFile(t, filepath.Join(wt, "scratch.txt"), "half-written, as the agent was told to carry on")
	if err := e.ExecuteRun(t.Context(), "repo", r.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}

	got, _, _ := ps.GetRun(r.ID)
	if got.Status != "passed" {
		out, _ := ps.RunOutput(r.ID)
		t.Fatalf("status = %q, want passed — the gate read the agent's live tree, not %s:\n%s",
			got.Status, shortSHA(r.Commit), out)
	}
	if _, passed, ok := ps.GateVerdict(r.Commit, "./check.sh"); !ok || !passed {
		t.Errorf("verdict for %s: ok=%v passed=%v, want the pass filed under the commit measured", shortSHA(r.Commit), ok, passed)
	}
	if head, _ := git.Head(wt); head != r.Commit {
		t.Errorf("the agent's branch moved during its own check: %q, want %q", head, r.Commit)
	}
	if _, err := os.Stat(filepath.Join(wt, "scratch.txt")); err != nil {
		t.Error("the work the agent did while waiting must be untouched")
	}
	if _, err := os.Stat(filepath.Join(root, ".worktrees", "gate")); err == nil {
		t.Error("the materialised checkout must be cleaned up, or the next gate inherits it")
	}
}

// TestASelfCheckIsQueuedNotRunInTheHub is the contention this is about: N agents linting at once used
// to mean N builds and test suites on the host, competing with the queued gates. The reply comes back
// at once with a position, and the gate itself waits its turn in the fleet's one slot.
func TestASelfCheckIsQueuedNotRunInTheHub(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "sd-1", Branch: "sd-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".worktrees", "bombur", "new.txt"), "work")

	out, code := lintVerb(t, e, "bombur")
	if code != 0 {
		t.Fatalf("lint = %d: %q", code, out)
	}
	if !strings.Contains(out, "queued at position 1") {
		t.Errorf("the reply should say where the gate sits, got %q", out)
	}
	runs, err := ps.Runs("queued")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Kind != gateLint {
		t.Fatalf("queued runs = %+v, want exactly one lint gate", runs)
	}
	if runs[0].Commit == "" {
		t.Error("a queued gate must name the commit it will check")
	}
	// A self-check parks nothing: the agent asked for it and carries on working while it waits.
	if st, _ := ps.GetState("bombur"); st.Phase != "working" {
		t.Errorf("phase = %q, want the agent left working — only a landing verb parks one", st.Phase)
	}
}

// TestASelfCheckOnAnUnchangedWorkspaceAnswersFromTheStore: the second ask must not pay for the gate
// again — and must say it did not, since a reused pass that reads like a fresh one is how an hour
// gets lost later, chasing a check that never happened.
func TestASelfCheckOnAnUnchangedWorkspaceAnswersFromTheStore(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	writeFile(t, filepath.Join(root, ".worktrees", "bombur", "new.txt"), "work")

	first := openGate(t, e, "bombur", gateLint, "")
	if err := e.ExecuteRun(t.Context(), "repo", first.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}

	out, code := lintVerb(t, e, "bombur")
	if code != 0 {
		t.Fatalf("lint = %d: %q", code, out)
	}
	if !strings.Contains(out, "reused") || !strings.Contains(out, "PASS") {
		t.Errorf("the answer should be the stored pass, said to be reused: %q", out)
	}
	if strings.Count(out, "gate PASS") != 1 {
		t.Errorf("the verdict is headed once, not once per reader: %q", out)
	}
	if runs, _ := ps.Runs("queued"); len(runs) != 0 {
		t.Errorf("nothing should be queued for a commit that already has a verdict, got %+v", runs)
	}
}

// TestAFailedSelfCheckLeavesTheAgentWorking: only the landing verbs park an agent in "gating". A
// self-check never did, so its result must not write a phase — it would overwrite whatever the agent
// moved on to while the gate waited.
func TestAFailedSelfCheckLeavesTheAgentWorking(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "sd-1", Branch: "sd-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".worktrees", "bombur", "new.txt"), "work")
	r := openGate(t, e, "bombur", gateLint, "")

	if err := e.completeGate("repo", r, "failed", "lint: line too long"); err != nil {
		t.Fatalf("completeGate: %v", err)
	}
	if st, _ := ps.GetState("bombur"); st.Phase != "working" || st.Task != "sd-1" {
		t.Errorf("state = %+v, want the agent left working on sd-1", st)
	}
	if _, exists, _ := ps.GetPR("pr-sd-1"); exists {
		t.Error("a self-check must never open a PR")
	}
	deps := e.deps.(*stubDeps)
	if len(deps.injectedText) == 0 || !strings.Contains(deps.injectedText[len(deps.injectedText)-1], "line too long") {
		t.Errorf("the agent must be told what failed, got %v", deps.injectedText)
	}
}

// TestACoauthorsCheckCommitsNothing: a coauthor's workspace IS the user's own checkout. Recording it
// would commit whatever the user has in progress under them, and a verdict stored against the
// resulting commit would describe a tree that was dirty when it was checked — so neither happens.
func TestACoauthorsCheckCommitsNothing(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	if err := ps.PutAgent(store.Agent{Name: "loki", Role: "coauthor", Workspace: "."}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "the-users-work.txt"), "half-finished")
	before, err := git.Head(root)
	if err != nil {
		t.Fatal(err)
	}

	out, code := lintVerb(t, e, "loki")
	if code != 0 {
		t.Fatalf("lint = %d: %q", code, out)
	}
	if head, _ := git.Head(root); head != before {
		t.Errorf("the user's checkout was committed to: HEAD %q → %q", before, head)
	}
	if dirty, _ := git.HasChanges(root); !dirty {
		t.Error("the user's work in progress must still be theirs, uncommitted")
	}
	runs, _ := ps.Runs("queued")
	if len(runs) != 1 || runs[0].Commit != "" {
		t.Fatalf("queued runs = %+v, want one gate naming no commit", runs)
	}
	if err := e.ExecuteRun(t.Context(), "repo", runs[0].ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	if _, ok := ps.GatePassed("", ""); ok {
		t.Error("a check of an unrecorded tree must not be stored as a verdict about a commit")
	}
}

// TestAReviewersOwnCheckWritesNothingToTheBranch: a reviewer's workspace is a DETACHED checkout of
// somebody else's branch. Committing there would file work under a PR its author never wrote — so an
// untouched tree is named by its HEAD (and reuses the submit gate's pass), and a touched one is
// checked as it stands, recorded against no commit at all.
func TestAReviewersOwnCheckWritesNothingToTheBranch(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	writeFile(t, filepath.Join(root, ".worktrees", "bombur", "new.txt"), "work")
	submitted := openGate(t, e, "bombur", gateSubmit, "the work")
	if err := e.ExecuteRun(t.Context(), "repo", submitted.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	review := filepath.Join(root, ".worktrees", "rune")
	if err := git.WorktreeAdd(root, review, "sd-1"); err != nil { // detached, as assignReview leaves it
		t.Fatal(err)
	}
	if err := ps.PutAgent(store.Agent{Name: "rune", Role: "reviewer", Workspace: ".worktrees/rune"}); err != nil {
		t.Fatal(err)
	}

	out, code := lintVerb(t, e, "rune")
	if code != 0 {
		t.Fatalf("lint = %d: %q", code, out)
	}
	if !strings.Contains(out, "reused") {
		t.Errorf("an untouched review checkout is the gated commit, so its verdict stands: %q", out)
	}
	tip, err := git.BranchTip(root, "sd-1")
	if err != nil {
		t.Fatal(err)
	}
	if tip != submitted.Commit {
		t.Errorf("the branch moved under its author: %q, want %q", tip, submitted.Commit)
	}

	// Now the reviewer has touched its tree: the check is about that, and about no commit.
	writeFile(t, filepath.Join(review, "reviewer-scratch.txt"), "poking at it")
	if out, code = lintVerb(t, e, "rune"); code != 0 {
		t.Fatalf("lint = %d: %q", code, out)
	}
	if !strings.Contains(out, "as it stands") {
		t.Errorf("a touched tree must be described as itself, not as a commit: %q", out)
	}
	runs, _ := ps.Runs("queued")
	if len(runs) != 1 || runs[0].Commit != "" {
		t.Fatalf("queued runs = %+v, want one gate naming no commit", runs)
	}
	if tip, _ := git.BranchTip(root, "sd-1"); tip != submitted.Commit {
		t.Error("the reviewer's own edit must never reach the branch under review")
	}
}

// TestAPRCheckReusesTheSubmitGatesPass is the reviewer's side of the same coin: the branch was gated
// minutes ago at exactly this commit, so pressing L must answer now rather than rebuild it.
func TestAPRCheckReusesTheSubmitGatesPass(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	writeFile(t, filepath.Join(root, ".worktrees", "bombur", "new.txt"), "work")
	r := openGate(t, e, "bombur", gateSubmit, "the work")
	if err := e.ExecuteRun(t.Context(), "repo", r.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	if _, exists, _ := ps.GetPR("pr-sd-1"); !exists {
		t.Fatal("precondition: the submit gate should have opened the PR")
	}

	out, err := e.LintPR("repo", "pr-sd-1")
	if err != nil {
		t.Fatalf("LintPR: %v", err)
	}
	if !strings.Contains(out, "PASS") || !strings.Contains(out, "reused") {
		t.Errorf("the PR check should reuse the submit gate's verdict, got %q", out)
	}
	if runs, _ := ps.Runs("queued"); len(runs) != 0 {
		t.Errorf("nothing should be queued to re-answer it, got %+v", runs)
	}
	if _, sha, _ := ps.GetPRLint("pr-sd-1"); sha != r.Commit {
		t.Errorf("stored against %q, want the commit gated (%q)", sha, r.Commit)
	}
}

// TestAPRCheckOnAMovedBranchIsQueuedOnce: with no verdict for the current tip the check must run —
// but asking twice must not queue twice, or the second run holds the fleet's only slot to compute a
// result the first is already on its way to.
func TestAPRCheckOnAMovedBranchIsQueuedOnce(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	wt := filepath.Join(root, ".worktrees", "bombur")
	writeFile(t, filepath.Join(wt, "new.txt"), "work")
	if _, err := e.gateCommit("repo", "bombur", "the work"); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-sd-1", Task: "sd-1", Agent: "bombur", Branch: "sd-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}

	first, err := e.LintPR("repo", "pr-sd-1")
	if err != nil {
		t.Fatalf("LintPR: %v", err)
	}
	if !strings.Contains(first, "gate queued") {
		t.Errorf("an unchecked commit must be queued, got %q", first)
	}
	if _, err := e.LintPR("repo", "pr-sd-1"); err != nil {
		t.Fatalf("second LintPR: %v", err)
	}
	runs, _ := ps.Runs("queued")
	if len(runs) != 1 {
		t.Fatalf("queued runs = %d, want the second ask to join the first", len(runs))
	}
	if runs[0].Agent != api.SenderUser {
		t.Errorf("run agent = %q, want the human's own, so it leads the queue", runs[0].Agent)
	}
	// The stored result is the wait itself, so the PR view shows it too — not only the pane of
	// whoever pressed the key.
	if stored, _, _ := ps.GetPRLint("pr-sd-1"); !strings.Contains(stored, "gate queued") {
		t.Errorf("stored lint = %q, want the queued state visible on the PR", stored)
	}

	// A reviewer asks third and JOINS that run. It cannot watch the board, so joining must record it
	// as someone to tell — otherwise it is parked on a message nobody sends, holding a verdict it was
	// told to have before deciding.
	if err := ps.PutAgent(store.Agent{Name: "rune", Role: "reviewer", Workspace: ".worktrees/rune"}); err != nil {
		t.Fatal(err)
	}
	if out, code := lintVerb(t, e, "rune", "pr-sd-1"); code != 0 {
		t.Fatalf("reviewer's lint = %d: %q", code, out)
	}
	if runs, _ := ps.Runs("queued"); len(runs) != 1 {
		t.Fatalf("queued runs = %d, want the reviewer to have joined the one already waiting", len(runs))
	}
	if again, _, _ := ps.GetRun(runs[0].ID); again.Agent != api.SenderUser {
		t.Errorf("run agent = %q, want the human who queued it kept — their run leads the queue", again.Agent)
	}
	deps := e.deps.(*stubDeps)
	before := len(deps.injected)
	if err := e.completeGate("repo", runs[0], "passed", "gate PASS · abc1234\n"); err != nil {
		t.Fatalf("completeGate: %v", err)
	}
	told := false
	for _, name := range deps.injected[before:] {
		told = told || name == "rune"
	}
	if !told {
		t.Errorf("the reviewer that joined was never told the gate landed (told: %v)", deps.injected[before:])
	}
}

// TestAFailedPRVerdictIsReadBackWithoutAnotherGate: the message that tells a reviewer its check has
// landed points at reading it, and a reader that must re-run a gate to see why one failed pays for a
// whole build and test — in the fleet's single slot — every time it looks. So a recorded verdict reads
// back, pass or fail; what may never stand on a stored failure is a DECISION, and reading is not one.
func TestAFailedPRVerdictIsReadBackWithoutAnotherGate(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	e.deps.(*stubDeps).projectConfig = config.Config{Verify: "./check.sh"}
	wt := filepath.Join(root, ".worktrees", "bombur")
	writeExec(t, filepath.Join(wt, "check.sh"), "#!/bin/sh\necho 'FAIL: the project says no'\nexit 1\n")
	r := openGate(t, e, "bombur", gateSubmit, "the work")
	if err := e.ExecuteRun(t.Context(), "repo", r.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	if got, _, _ := ps.GetRun(r.ID); got.Status != "failed" {
		t.Fatalf("precondition: the gate should have failed, got %q", got.Status)
	}
	// The PR is put up by hand: a failed gate creates none, and it is the reviewer's read that is
	// under test here, not the landing.
	if err := ps.PutPR(store.PR{ID: "pr-sd-1", Task: "sd-1", Agent: "bombur", Branch: "sd-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}

	out, err := e.LintPR("repo", "pr-sd-1")
	if err != nil {
		t.Fatalf("LintPR: %v", err)
	}
	if !strings.Contains(out, "FAIL") || !strings.Contains(out, "the project says no") {
		t.Errorf("the reader should get the recorded failure itself: %q", out)
	}
	if !strings.Contains(out, "reused") {
		t.Errorf("and must be told nothing re-ran: %q", out)
	}
	if runs, _ := ps.Runs("queued"); len(runs) != 0 {
		t.Errorf("reading a recorded verdict must queue nothing, got %+v", runs)
	}
	// The rule that matters is untouched: a DECISION never stands on a stored failure.
	if _, ok := ps.GatePassed(r.Commit, "./check.sh"); ok {
		t.Error("a stored failure must never answer a landing decision")
	}
}

// TestAReusedPassNeverSitsQueued: the row is written already settled, so the run watcher cannot dequeue
// and execute for real what the reuse path is about to finish — which would land one submit's
// continuation twice, with the phase decided by whichever finished last.
func TestAReusedPassNeverSitsQueued(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	writeFile(t, filepath.Join(root, ".worktrees", "bombur", "new.txt"), "work")
	first := openGate(t, e, "bombur", gateLint, "")
	if err := e.ExecuteRun(t.Context(), "repo", first.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}

	sha, err := e.gateCommit("repo", "bombur", "")
	if err != nil {
		t.Fatal(err)
	}
	run, reused, err := e.gateRun("repo", "bombur", gateSubmit, "", sha)
	if err != nil || !reused {
		t.Fatalf("reused = %v (err %v), want the stored pass reused", reused, err)
	}
	if got, _, _ := ps.GetRun(run.ID); got.Status != "passed" {
		t.Errorf("status = %q, want the row settled at once", got.Status)
	}
	if _, _, ok := e.NextQueuedRun(); ok {
		t.Error("the watcher has something to dequeue — a reused gate must never look runnable")
	}
}

// TestAPRCheckGatesTheBranchNotTheAuthorsTree: the check used to run in the author's worktree, which
// after a submit holds whatever it has started since — so a reviewer could be shown a verdict about
// code that is not in the PR. The commit the branch names is the only honest subject.
func TestAPRCheckGatesTheBranchNotTheAuthorsTree(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	wt := filepath.Join(root, ".worktrees", "bombur")
	writeFile(t, filepath.Join(wt, "new.txt"), "work")
	r := openGate(t, e, "bombur", gateSubmit, "the work")
	if err := e.ExecuteRun(t.Context(), "repo", r.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	// The author moves on: a change in its tree that no commit on the branch carries.
	writeFile(t, filepath.Join(wt, "unrelated.txt"), "next task's work")

	run, err := e.putRun("repo", store.Run{
		Agent: api.SenderUser, Kind: gateLintPR, Message: "pr-sd-1", Commit: r.Commit, Command: "gate: lint pr-sd-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.ExecuteRun(t.Context(), "repo", run.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	got, _, _ := ps.GetRun(run.ID)
	if got.Commit != r.Commit {
		t.Errorf("checked %q, want the branch's own commit %q", got.Commit, r.Commit)
	}
	if dirty, _ := git.HasChanges(wt); !dirty {
		t.Error("the author's own tree must be left exactly as it was, uncommitted work and all")
	}
	if _, err := git.Head(filepath.Join(root, ".worktrees", "gate")); err == nil {
		t.Error("the materialised tree must be cleaned up, or the next check inherits it")
	}
}

// TestAPrecheckWaitsBehindAnAgentsGate: the periodic re-check of an open PR builds and tests like any
// other gate, but nobody is held up by it — an agent parked on a submit is. So it goes through the
// same single slot, and behind.
func TestAPrecheckWaitsBehindAnAgentsGate(t *testing.T) {
	e, ps, root := gateRepo(t, "bombur", "sd-1")
	writeFile(t, filepath.Join(root, ".worktrees", "bombur", "new.txt"), "work")

	precheck, err := e.putRun("repo", store.Run{
		Agent: api.SenderSystem, Kind: gatePrecheck, Message: "pr-sd-1", Command: "gate: precheck pr-sd-1 onto main",
	})
	if err != nil {
		t.Fatal(err)
	}
	submit := openGate(t, e, "bombur", gateSubmit, "the work")

	all, err := e.store.AllRuns("queued")
	if err != nil {
		t.Fatal(err)
	}
	pos := queuePositions(all)
	if pos[submit.ID] >= pos[precheck.ID] {
		t.Errorf("submit=%d precheck=%d — the agent waiting on a verdict goes first", pos[submit.ID], pos[precheck.ID])
	}
	if _, _, ranAt := ps.GetPRLint("pr-sd-1"); ranAt != "" {
		t.Error("an advisory check must not overwrite the PR's own gate result")
	}
}
