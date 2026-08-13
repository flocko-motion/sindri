package workflow

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

func TestRunTimeoutClampsToHardCap(t *testing.T) {
	cases := map[string]time.Duration{
		"":        RunHardCap,
		"garbage": RunHardCap,
		"0s":      RunHardCap,
		"-1m":     RunHardCap,
		"30m":     RunHardCap, // over the cap
		"5m":      5 * time.Minute,
		"90s":     90 * time.Second,
	}
	for in, want := range cases {
		if got := runTimeout(in); got != want {
			t.Errorf("runTimeout(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestRunCacheMountsCreatesDirsAndPointsGoAtThem(t *testing.T) {
	t.Setenv("SINDRI_HOME", t.TempDir())
	mounts, env, err := runCacheMounts("repo")
	if err != nil {
		t.Fatalf("runCacheMounts: %v", err)
	}
	if len(mounts) != 4 {
		t.Fatalf("got %d mounts, want 4 (go-build, go-mod, node_modules, target)", len(mounts))
	}
	for _, m := range mounts {
		if _, err := os.Stat(m.Host); err != nil {
			t.Errorf("cache dir %s not created: %v", m.Host, err)
		}
		if m.Mode != "rw" {
			t.Errorf("cache mount %s mode = %q, want rw — the whole point is writing to it", m.Container, m.Mode)
		}
	}
	if env["GOCACHE"] == "" || env["GOMODCACHE"] == "" {
		t.Errorf("env missing GOCACHE/GOMODCACHE: %+v", env)
	}
}

// TestExecuteRunDropsAStaleRunWithNoAgent covers the case ExecuteRun must reject before ever
// touching a container: the run's scheduling agent no longer exists (retired/deleted mid-queue).
// This is a drop, not a failure of the command — the run never got a chance to say anything about
// itself, so "cancelled" is the honest status.
func TestExecuteRunDropsAStaleRunWithNoAgent(t *testing.T) {
	e, ps := runEngine(t)
	r, err := e.ScheduleRun("repo", "ghost", "go test ./...", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.ExecuteRun("repo", r.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	got, _, _ := ps.GetRun(r.ID)
	if got.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", got.Status)
	}
	if got.ExitCode != -1 {
		t.Errorf("exit code = %d, want -1 (nothing ran)", got.ExitCode)
	}
	out, _ := ps.RunOutput(r.ID)
	if !strings.Contains(out, "ghost") {
		t.Errorf("output should name the missing agent: %q", out)
	}
}

// TestExecuteRunDropsAStaleRunWhenTheAgentMovedOn: the agent scheduled this while holding task
// td-1, but by the time it reached the front it holds td-2 — the workspace this run was meant to
// test no longer exists there, so it must be dropped rather than spend the only slot on it.
func TestExecuteRunDropsAStaleRunWhenTheAgentMovedOn(t *testing.T) {
	e, ps := runEngine(t)
	if err := ps.PutAgent(store.Agent{Name: "bombur", Role: "worker", Workspace: ".worktrees/bombur"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "td-1"}); err != nil {
		t.Fatal(err)
	}
	r, err := e.ScheduleRun("repo", "bombur", "go test ./...", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Task != "td-1" {
		t.Fatalf("ScheduleRun should snapshot the agent's task: got %q, want td-1", r.Task)
	}
	if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "td-2"}); err != nil {
		t.Fatal(err)
	}
	if err := e.ExecuteRun("repo", r.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	got, _, _ := ps.GetRun(r.ID)
	if got.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", got.Status)
	}
	out, _ := ps.RunOutput(r.ID)
	if !strings.Contains(out, "moved on") {
		t.Errorf("output should explain why: %q", out)
	}
}

// TestExecuteRunSkipsAlreadySettledRuns: a run cancelled by the user between NextQueuedRun
// choosing it and ExecuteRun actually starting must not be executed anyway.
func TestExecuteRunSkipsAlreadySettledRuns(t *testing.T) {
	e, ps := runEngine(t)
	r, err := e.ScheduleRun("repo", "bombur", "go test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.SetRunStatus(r.ID, "cancelled"); err != nil {
		t.Fatal(err)
	}
	if err := e.ExecuteRun("repo", r.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	got, _, _ := ps.GetRun(r.ID)
	out, _ := ps.RunOutput(r.ID)
	if got.Status != "cancelled" || out != "" {
		t.Fatalf("an already-settled run must be left alone: %+v, output %q", got, out)
	}
}

// TestExecuteRunMaterializesThenFailsWithoutARuntime exercises the real path — a live agent with a
// worktree on disk — up to the container port, which is unwired (noop) in every test process. This
// is the deterministic graceful-failure path this sandbox can exercise; the happy path needs a real
// podman/apple-container backend.
func TestExecuteRunMaterializesThenFailsWithoutARuntime(t *testing.T) {
	e, ps := runEngine(t)
	deps := e.deps.(*stubDeps)
	wt := filepath.Join(deps.root, ".worktrees", "bombur")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "uncommitted.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutAgent(store.Agent{Name: "bombur", Role: "worker", Workspace: ".worktrees/bombur"}); err != nil {
		t.Fatal(err)
	}
	r, err := e.ScheduleRun("repo", "bombur", "go test ./...", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.ExecuteRun("repo", r.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	got, _, _ := ps.GetRun(r.ID)
	if got.Status != "failed" {
		t.Fatalf("status = %q, want failed (no container runtime wired in tests)", got.Status)
	}
	if got.StartedAt != "" {
		t.Error("image preparation failed before the run was ever marked running — started_at should stay empty")
	}
	out, _ := ps.RunOutput(r.ID)
	if !strings.Contains(out, "image") {
		t.Errorf("output should explain the image step failed: %q", out)
	}
	// The materialized copy is always removed, success or failure.
	if _, err := os.Stat(filepath.Join(deps.root, ".worktrees", "run-"+r.ID)); err == nil {
		t.Error("materialized run workspace should have been cleaned up")
	}
}

func TestNextQueuedRunReturnsHighestRanked(t *testing.T) {
	e, _ := runEngine(t)
	if _, _, ok := e.NextQueuedRun(); ok {
		t.Fatal("empty queue must report ok=false")
	}
	low, err := e.ScheduleRun("repo", "bombur", "go test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	high, err := e.ScheduleRun("repo", "nori", "go build", "P0", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = low
	project, id, ok := e.NextQueuedRun()
	if !ok || project != "repo" || id != high.ID {
		t.Errorf("NextQueuedRun = (%q, %q, %v), want (repo, %q, true)", project, id, ok, high.ID)
	}
}

func TestMsgRunFinished(t *testing.T) {
	got := MsgRunFinished("run-abc", "passed", 90*time.Second, 5*time.Minute)
	if !strings.Contains(got, "run-abc") || !strings.Contains(got, "passed") {
		t.Errorf("summary missing id/status: %q", got)
	}
	if !strings.Contains(got, "1m30s") || !strings.Contains(got, "5m0s") {
		t.Errorf("summary should report budget usage: %q", got)
	}
	if strings.Contains(got, "\n\n") {
		t.Errorf("a summary is meant to be short, not a multi-paragraph injection: %q", got)
	}
	timedOut := MsgRunFinished("run-abc", "timed_out", 15*time.Minute, 15*time.Minute)
	if !strings.Contains(timedOut, "timed out") {
		t.Errorf("timed_out should read as a human phrase: %q", timedOut)
	}
}

func TestCmdShowDispatchesRunsToCmdShowRun(t *testing.T) {
	e, _ := runEngine(t)
	r, err := e.ScheduleRun("repo", "bombur", "go test ./...", "", "")
	if err != nil {
		t.Fatal(err)
	}
	c := registry.Caller{Project: "repo", Agent: "bombur", Role: "worker"}
	var out strings.Builder
	code, err := e.CmdShow(c, []string{r.ID}, &out)
	if err != nil || code != 0 {
		t.Fatalf("CmdShow(%q): code=%d err=%v", r.ID, code, err)
	}
	if !strings.Contains(out.String(), r.ID) || !strings.Contains(out.String(), "queued") {
		t.Errorf("CmdShow should print the run's id and status: %q", out.String())
	}

	out.Reset()
	code, err = e.CmdShow(c, []string{"no-such-pr"}, &out)
	if err == nil {
		t.Fatalf("a non-run id with no matching PR should error, got code=%d", code)
	}
}

func TestRunCancelSet(t *testing.T) {
	var s runCancelSet
	if s.consume("run-x") {
		t.Fatal("nothing was requested yet")
	}
	s.request("run-x")
	if !s.consume("run-x") {
		t.Fatal("a requested id must be reported")
	}
	if s.consume("run-x") {
		t.Fatal("consume must clear it — a second read must not see the same request again")
	}
}

func TestExitCodeOf(t *testing.T) {
	if got := exitCodeOf(nil); got != -1 {
		t.Errorf("exitCodeOf(nil) = %d, want -1 (no error is not this function's job to see)", got)
	}
	if got := exitCodeOf(errors.New("boom")); got != -1 {
		t.Errorf("a non-exec error should report -1, got %d", got)
	}
	cmd := exec.Command("sh", "-c", "exit 7")
	err := cmd.Run()
	if got := exitCodeOf(err); got != 7 {
		t.Errorf("exitCodeOf(exit 7) = %d, want 7", got)
	}
}

// TestCancelRunKillsARunningContainerWithoutWritingItsStatus: cancelling a running run must not
// itself write the terminal status — that races ExecuteRun's own finish. It records the kill
// request (for the executing goroutine to notice) and best-effort removes the container.
func TestCancelRunKillsARunningContainerWithoutWritingItsStatus(t *testing.T) {
	e, ps := runEngine(t)
	r, err := e.ScheduleRun("repo", "bombur", "go test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.SetRunStatus(r.ID, "running"); err != nil {
		t.Fatal(err)
	}
	if err := e.CancelRun("repo", r.ID); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	got, _, _ := ps.GetRun(r.ID)
	if got.Status != "running" {
		t.Fatalf("status = %q, want still running — ExecuteRun's own goroutine records the finish", got.Status)
	}
	if !e.runCancels.consume(r.ID) {
		t.Error("cancelling a running run must record the kill request")
	}
}

// TestReconcileRunningRunsFailsAnOrphan mirrors ReconcileMergingPRs' precedent: a run still
// "running" at hub startup has no container behind it any more, so it must not occupy the
// fleet's only slot forever.
func TestReconcileRunningRunsFailsAnOrphan(t *testing.T) {
	e, ps := runEngine(t)
	r, err := e.ScheduleRun("repo", "bombur", "go test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.SetRunStatus(r.ID, "running"); err != nil {
		t.Fatal(err)
	}
	e.ReconcileRunningRuns()
	got, _, _ := ps.GetRun(r.ID)
	if got.Status != "failed" {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	out, _ := ps.RunOutput(r.ID)
	if !strings.Contains(out, "restart") {
		t.Errorf("output should explain it was orphaned by a restart: %q", out)
	}
}
