// package: hub/workflow / execrun
// type:    logic (execute one queued run)
// job:     run one queued run to completion — fresh capped container, materialized worktree,
// build cache, 15-minute cap — plus cancelling one in flight and reconciling one
// orphaned by a hub restart.
// limits:  one run, synchronously; picking the next and one-at-a-time are the caller's
// (-> NextQueuedRun, hub/runwatch.go).
package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/repo"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/tools/paths"
)

// RunHardCap bounds every run regardless of what it asks for. At concurrency one, a hung run
// starves every agent behind it in the queue, so this is what stops one bad test wedging the fleet.
const RunHardCap = 15 * time.Minute

// runRemoveTimeout bounds tearing a run's container down once the command is finished or its budget
// spent. Its own bound because `rm -f` stops before it removes, so it is the slowest verb here.
const runRemoveTimeout = 30 * time.Second

// runTimeout resolves a run's requested budget against the hard cap: unset, unparsable, or over
// cap all fall back to the cap itself — a request can only narrow it, never widen it.
func runTimeout(requested string) time.Duration {
	d, err := time.ParseDuration(requested)
	if err != nil || d <= 0 || d > RunHardCap {
		return RunHardCap
	}
	return d
}

// ExecuteRun runs a queued run to completion, always leaving it in a terminal status with output
// on record. A returned error means the run was never attempted at all.
func (e *Engine) ExecuteRun(ctx context.Context, project, id string) error {
	ps := e.store.For(project)
	r, ok, err := ps.GetRun(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such run %q", id)
	}
	if r.Status != "queued" {
		return nil // already settled (e.g. cancelled) before this reached the front
	}
	if reason := e.staleReason(ps, r); reason != "" {
		return e.finishRun(ps, project, r, "cancelled", "run: dropped before executing — "+reason+"\n", 0, 0, -1)
	}
	if r.Kind != "" {
		return e.executeGateRun(ctx, ps, project, r)
	}

	root := e.deps.ProjectRoot(project)
	// The workspace the run was AIMED at, from its own row: re-deriving it from the agent answered
	// differently once it had moved on, and answers nothing at all for a run the user queued.
	mwt, err := repo.MaterializeRun(root, filepath.Join(root, r.Workspace), id)
	if err != nil {
		return e.finishRun(ps, project, r, "failed", fmt.Sprintf("run: could not prepare a workspace: %s\n", err), 0, 0, -1)
	}
	defer func() { _ = repo.RemoveRunMaterialization(mwt) }()

	cfg, err := e.deps.ProjectConfig(project)
	if err != nil {
		return e.finishRun(ps, project, r, "failed", fmt.Sprintf("run: could not read project config: %s\n", err), 0, 0, -1)
	}
	imageRef, err := container.EnsureImage(root, config.Abs(root, cfg.Containerfile), io.Discard)
	if err != nil {
		return e.finishRun(ps, project, r, "failed", fmt.Sprintf("run: could not prepare the image: %s\n", err), 0, 0, -1)
	}
	cacheMounts, cacheEnv, err := runCacheMounts(project)
	if err != nil {
		return e.finishRun(ps, project, r, "failed", fmt.Sprintf("run: could not prepare the build cache: %s\n", err), 0, 0, -1)
	}

	if err := ps.SetRunStatus(id, "running"); err != nil {
		return err
	}
	e.deps.Notify()

	name := e.hn.Container(project, "run-"+id)
	mounts := append([]container.Mount{{Host: mwt, Container: "/workspace", Mode: "rw"}}, cacheMounts...)
	opts := container.RunOpts{
		Name:       name,
		Image:      imageRef,
		Labels:     map[string]string{"sindri.project": root, "sindri.run": id},
		Env:        cacheEnv,
		Mounts:     mounts,
		Workdir:    "/workspace",
		Entrypoint: []string{"sleep", "infinity"}, // kept alive so ExecContext can run the command below
		Memory:     container.DefaultMemory(),     // one limit to tune, the same as an agent's default (-> agent.MemoryOrDefault)
	}
	if err := container.Run(opts); err != nil {
		return e.finishRun(ps, project, r, "failed", fmt.Sprintf("run: could not start a container: %s\n", err), 0, 0, -1)
	}
	budget := runTimeout(r.Timeout)
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	// After the bound, and with a context of its own: the removal is this run's last act, and a
	// budget already spent (or a hub shutting down) must not take the teardown with it.
	defer func() {
		rmCtx, rmCancel := context.WithTimeout(context.WithoutCancel(ctx), runRemoveTimeout)
		defer rmCancel()
		_ = container.RmContext(rmCtx, name)
	}()
	start := time.Now()
	out, execErr := container.ExecContext(ctx, name, "sh", "-c", r.Command)
	elapsed := time.Since(start)

	status, exitCode := "passed", 0
	switch {
	case e.runCancels.consume(id):
		status, exitCode = "cancelled", -1
	case ctx.Err() == context.DeadlineExceeded:
		status, exitCode = "timed_out", -1
	case execErr != nil:
		status, exitCode = "failed", exitCodeOf(execErr)
	}
	output := string(out)
	switch status {
	case "timed_out":
		output += fmt.Sprintf("\nrun: exceeded its %s budget and was stopped — the container is gone; nothing from it keeps running.\n", budget)
	case "cancelled":
		output += "\nrun: cancelled while executing — the container was removed.\n"
	}
	return e.finishRun(ps, project, r, status, output, elapsed, budget, exitCode)
}

// exitCodeOf reports a failed exec's process exit code, or -1 when none is available (the runtime
// itself failed to run the command at all, rather than the command running and exiting nonzero).
func exitCodeOf(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// staleReason reports why a queued run should be dropped rather than executed, or "" if it is
// still good — cheaper to check than to materialize, build, and run against a workspace the
// scheduling agent has since left behind.
func (e *Engine) staleReason(ps *store.ProjectStore, r api.Run) string {
	// Nothing to go stale: no roster entry, no task. Dropping one for a missing "agent" named user
	// would silently discard the run a human is sitting there waiting for, and a gate on a PR names
	// its subject, which is checked when it executes (-> gateTree).
	if api.RunFromUser(r) || gateOnAPR(r) {
		return ""
	}
	_, ok, err := ps.GetAgent(r.Agent)
	if err != nil || !ok {
		return fmt.Sprintf("agent %s no longer exists", r.Agent)
	}
	if st, err := ps.GetState(r.Agent); err == nil && r.Task != "" && st.Task != r.Task {
		return fmt.Sprintf("agent %s has moved on to a different task since this was queued", r.Agent)
	}
	return ""
}

// finishRun records a run's terminal outcome — status, full uncapped output, exit code. A gate
// run's result unlocks its submit/contribute continuation (-> completeGate); an ordinary run just
// gets a summary injected, never the full log (-> MsgRunFinished).
func (e *Engine) finishRun(ps *store.ProjectStore, project string, r api.Run, status, output string, elapsed, budget time.Duration, exitCode int) error {
	if err := ps.SetRunResult(r.ID, status, output, exitCode); err != nil {
		return err
	}
	e.deps.Notify()
	if r.Kind != "" {
		return e.completeGate(project, r, status, output)
	}
	// Nobody to deliver to: the user reads it on the board, which Notify has already refreshed.
	if api.RunFromUser(r) {
		return nil
	}
	_ = e.hn.Say(project, r.Agent, MsgRunFinished(r.ID, status, elapsed, budget), MailAndPush)
	return nil
}

// runCancelSet tracks run ids killed mid-execution, so the blocked ExecContext call can tell a
// kill from an ordinary failure without a second write racing ExecuteRun's own finish.
type runCancelSet struct {
	mu  sync.Mutex
	ids map[string]bool
}

func (s *runCancelSet) request(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ids == nil {
		s.ids = map[string]bool{}
	}
	s.ids[id] = true
}

// consume reports whether id was requested, clearing it either way so the set never grows past
// however many runs are cancelled and not yet finished (at most one, at concurrency one).
func (s *runCancelSet) consume(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	ok := s.ids[id]
	delete(s.ids, id)
	return ok
}

// CancelRun withdraws a queued run, or kills a running one's container so the slot frees now
// rather than waiting out its timeout — leaving ExecuteRun's own goroutine to record the finish.
func (e *Engine) CancelRun(ctx context.Context, project, id string) error {
	ps := e.store.For(project)
	r, ok, err := ps.GetRun(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such run %q", id)
	}
	if !api.RunOpen(r) {
		return fmt.Errorf("%s is %s — already finished, nothing to cancel", id, r.Status)
	}
	if r.Status == "running" {
		e.runCancels.request(id)
		rmCtx, rmCancel := context.WithTimeout(context.WithoutCancel(ctx), runRemoveTimeout)
		defer rmCancel()
		_ = container.RmContext(rmCtx, e.hn.Container(project, "run-"+id))
		return nil
	}
	if err := ps.SetRunStatus(id, "cancelled"); err != nil {
		return err
	}
	e.deps.Notify()
	return nil
}

// ReconcileRunningRuns runs at hub startup: a run still "running" was orphaned by the last hub
// dying mid-execution — the ReconcileMergingPRs precedent, applied here so a ghost never sits on
// the fleet's only slot forever.
func (e *Engine) ReconcileRunningRuns(ctx context.Context) {
	runs, err := e.store.AllRuns("running")
	if err != nil {
		log.Printf("hub: reconcile running runs: %v", err)
		return
	}
	for _, r := range runs {
		ps := e.store.For(r.Project)
		rmCtx, rmCancel := context.WithTimeout(context.WithoutCancel(ctx), runRemoveTimeout)
		_ = container.RmContext(rmCtx, e.hn.Container(r.Project, "run-"+r.ID))
		rmCancel()
		// "cancelled", not "failed": nothing here found a violation or a broken build, and a gate
		// run reconciled this way must not read as one either (-> stallGate, not rejectGate).
		note := "run: hub restarted mid-run — outcome unknown; its container has been removed.\n"
		if err := e.finishRun(ps, r.Project, r, "cancelled", note, 0, 0, -1); err != nil {
			log.Printf("hub: reconcile run %s: %v", r.ID, err)
			continue
		}
		log.Printf("hub: %s was running at restart → cancelled (outcome unknown)", r.ID)
	}
}

// runCacheMounts returns a run's persistent build-cache mounts and the Go env pointing at them,
// creating the host directories on demand — safe to share unguarded since only one run ever
// executes at a time.
func runCacheMounts(project string) ([]container.Mount, map[string]string, error) {
	base := filepath.Join(paths.StateDir(), project, "run-cache")
	specs := []struct{ dir, container string }{
		{"go-build", "/cache/go-build"},
		{"go-mod", "/cache/go-mod"},
		{"node-modules", "/workspace/node_modules"},
		{"target", "/workspace/target"},
	}
	mounts := make([]container.Mount, 0, len(specs))
	for _, s := range specs {
		host := filepath.Join(base, s.dir)
		if err := os.MkdirAll(host, 0o755); err != nil {
			return nil, nil, fmt.Errorf("run cache dir %s: %w", s.dir, err)
		}
		mounts = append(mounts, container.Mount{Host: host, Container: s.container, Mode: "rw"})
	}
	return mounts, map[string]string{"GOCACHE": "/cache/go-build", "GOMODCACHE": "/cache/go-mod"}, nil
}
