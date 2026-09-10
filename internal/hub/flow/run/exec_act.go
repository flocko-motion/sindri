// package: hub/flow/run / exec_act
// type:    logic (what the run flow's actions do)
// job:     run one run to completion — fresh capped container, materialized worktree, build cache,
// 15-minute cap — settle one nobody wants any more, and cancel one in flight.
// limits:  performing. WHICH run runs, and when, is the map's (-> hub/flow/run): the front of the
// one fleet queue with the slot free is a condition there, not a mutex here.
package run

import (
	"context"
	"errors"
	"fmt"
	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
	"github.com/flo-at/sindri/internal/hub/world/store"
	"github.com/flo-at/sindri/internal/tools/paths"
)

// RunHardCap bounds every run regardless of what it asks for. At concurrency one, a hung run
// starves every agent behind it in the queue, so this is what stops one bad test wedging the fleet.
const RunHardCap = 15 * time.Minute

// runRemoveTimeout bounds tearing a run's container down once the command is finished or its budget
// spent. Its own bound because `rm -f` stops before it removes, so it is the slowest verb here.
const runRemoveTimeout = 30 * time.Second

// RunTimeout resolves a run's requested budget against the hard cap: unset, unparsable, or over
// cap all fall back to the cap itself — a request can only narrow it, never widen it.
func RunTimeout(requested string) time.Duration {
	d, err := time.ParseDuration(requested)
	if err != nil || d <= 0 || d > RunHardCap {
		return RunHardCap
	}
	return d
}

// ExecuteRun runs a queued run to completion, always leaving it in a terminal status with output
// on record. A returned error means the run was never attempted at all.
func (a *Act) ExecuteRun(ctx context.Context, project, id string) error {
	ps := a.Store.For(project)
	r, ok, err := ps.GetRun(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such run %q", id)
	}
	if !api.RunOpen(r) {
		return nil // already settled (a.g. cancelled) before this reached the front
	}
	if r.Kind != "" {
		return a.Gate.ExecuteGateRun(ctx, project, r)
	}

	root := a.Deps.ProjectRoot(project)
	// The workspace the run was AIMED at, from its own row: re-deriving it from the agent answered
	// differently once it had moved on, and answers nothing at all for a run the user queued.
	mwt, err := git.MaterializeRun(root, filepath.Join(root, r.Workspace), id)
	if err != nil {
		return a.FinishRun(ps, project, r, "failed", fmt.Sprintf("run: could not prepare a workspace: %s\n", err), 0, 0, -1)
	}
	defer func() { _ = git.RemoveRunMaterialization(mwt) }()

	cfg, err := a.Deps.ProjectConfig(project)
	if err != nil {
		return a.FinishRun(ps, project, r, "failed", fmt.Sprintf("run: could not read project config: %s\n", err), 0, 0, -1)
	}
	imageRef, err := container.EnsureImage(root, config.Abs(root, cfg.Containerfile), io.Discard)
	if err != nil {
		return a.FinishRun(ps, project, r, "failed", fmt.Sprintf("run: could not prepare the image: %s\n", err), 0, 0, -1)
	}
	cacheMounts, cacheEnv, err := RunCacheMounts(project)
	if err != nil {
		return a.FinishRun(ps, project, r, "failed", fmt.Sprintf("run: could not prepare the build cache: %s\n", err), 0, 0, -1)
	}

	name := a.Harness.Container(project, "run-"+id)
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
		return a.FinishRun(ps, project, r, "failed", fmt.Sprintf("run: could not start a container: %s\n", err), 0, 0, -1)
	}
	budget := RunTimeout(r.Timeout)
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
	case a.Kills.Consume(id):
		status, exitCode = "cancelled", -1
	case ctx.Err() == context.DeadlineExceeded:
		status, exitCode = "timed_out", -1
	case execErr != nil:
		status, exitCode = "failed", ExitCodeOf(execErr)
	}
	output := string(out)
	switch status {
	case "timed_out":
		output += fmt.Sprintf("\nrun: exceeded its %s budget and was stopped — the container is gone; nothing from it keeps running.\n", budget)
	case "cancelled":
		output += "\nrun: cancelled while executing — the container was removed.\n"
	}
	return a.FinishRun(ps, project, r, status, output, elapsed, budget, exitCode)
}

// ExitCodeOf reports a failed exec's process exit code, or -1 when none is available (the runtime
// itself failed to run the command at all, rather than the command running and exiting nonzero).
func ExitCodeOf(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// StaleReason reports why a queued run should be dropped rather than executed, or "" if it is
// still good — cheaper to check than to materialize, build, and run against a workspace the
// scheduling agent has since left behind.
func (a *Act) StaleReason(ps *store.ProjectStore, r api.Run) string {
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

// FinishRun records a run's terminal outcome — status, full uncapped output, exit code. A gate
// run's result unlocks its submit/contribute continuation (-> completeGate); an ordinary run just
// gets a summary injected, never the full log (-> prompts.MsgRunFinished).
func (a *Act) FinishRun(ps *store.ProjectStore, project string, r api.Run, status, output string, elapsed, budget time.Duration, exitCode int) error {
	if err := ps.SetRunResult(r.ID, status, output, exitCode); err != nil {
		return err
	}
	a.Deps.Notify()
	// The fleet has ONE slot, and this run just left it. Telling the queue at once is the difference
	// between the next run starting now and it waiting out a poll for a slot already free.
	a.Flow.WakeRuns(topic.GateFinished)
	a.Flow.WakeProject(project, topic.GateFinished)
	if r.Kind != "" {
		return a.Gate.CompleteGate(project, r, status, output)
	}
	// Nobody to deliver to: the user reads it on the board, which Notify has already refreshed.
	if api.RunFromUser(r) {
		return nil
	}
	_ = a.Harness.Say(project, r.Agent, prompts.MsgRunFinished(r.ID, status, elapsed, budget), mail.MailAndPush)
	return nil
}

// CancelRun withdraws a queued run, or kills a running one's container so the slot frees now
// rather than waiting out its timeout — leaving ExecuteRun's own goroutine to record the finish.
func (a *Act) CancelRun(ctx context.Context, project, id string) error {
	ps := a.Store.For(project)
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
		a.Kills.Request(id)
		rmCtx, rmCancel := context.WithTimeout(context.WithoutCancel(ctx), runRemoveTimeout)
		defer rmCancel()
		_ = container.RmContext(rmCtx, a.Harness.Container(project, "run-"+id))
		return nil
	}
	if err := ps.SetRunStatus(id, "cancelled"); err != nil {
		return err
	}
	a.Deps.Notify()
	return nil
}

// RunCacheMounts returns a run's persistent build-cache mounts and the Go env pointing at them,
// creating the host directories on demand — safe to share unguarded since only one run ever
// executes at a time.
func RunCacheMounts(project string) ([]container.Mount, map[string]string, error) {
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

// --- the run flow's actions (-> hub/flow/run) ---
// doExecuteRun is the executing state's action: the command runs and the row is left settled.
// Refused only when the workspace, image or cache could not be prepared; every else is a verdict.
func (a *Act) doExecuteRun(ctx context.Context, w World) (Outcome, error) {
	if err := a.ExecuteRun(ctx, w.Run.Project, w.Run.ID); err != nil {
		return Refused, err
	}
	// The agent behind a gate run is parked until this answers, so it is woken by name rather than
	// left to its own next beat. A topic, never a reach into its state: this module writes runs.
	if !api.RunFromUser(w.Run) {
		a.Flow.Wake(w.Run.Project, w.Run.Agent, topic.GateFinished)
	}
	return Ran, nil
}

// doDropRun settles a run nobody wants any more, writing WHY on the record. "cancelled", not
// "failed": nothing found a broken build, and a gate dropped this way must not read as one.
func (a *Act) doDropRun(_ context.Context, w World) (Outcome, error) {
	ps := a.Store.For(w.Run.Project)
	note := "run: dropped before executing — " + w.Stale + "\n"
	if err := a.FinishRun(ps, w.Run.Project, w.Run, "cancelled", note, 0, 0, -1); err != nil {
		return Dropped, err
	}
	return Dropped, nil
}

// ReconcileRunningRuns settles, at startup, every run left "running" by a hub that died. The map's
// Orphaned exit reaches them too; the fleet has ONE slot, so clearing a ghost at boot is worth it.
func (a *Act) ReconcileRunningRuns(ctx context.Context) {
	runs, err := a.Store.AllRuns("running")
	if err != nil {
		log.Printf("hub: reconcile running runs: %v", err)
		return
	}
	for _, r := range runs {
		if err := a.AbandonOrphanedRun(ctx, r.Project, r.ID); err != nil {
			log.Printf("hub: reconcile run %s: %v", r.ID, err)
			continue
		}
		log.Printf("hub: %s was running at restart → cancelled (outcome unknown)", r.ID)
	}
}

// AbandonOrphanedRun settles a run found already "running": a previous hub died holding the fleet's
// only slot. "cancelled", not "failed" — nothing here found a violation (-> stallGate).
func (a *Act) AbandonOrphanedRun(ctx context.Context, project, id string) error {
	ps := a.Store.For(project)
	r, ok, err := ps.GetRun(id)
	if err != nil || !ok {
		return err
	}
	rmCtx, rmCancel := context.WithTimeout(context.WithoutCancel(ctx), runRemoveTimeout)
	defer rmCancel()
	_ = container.RmContext(rmCtx, a.Harness.Container(project, "run-"+id))
	note := "run: hub restarted mid-run — outcome unknown; its container has been removed.\n"
	return a.FinishRun(ps, project, r, "cancelled", note, 0, 0, -1)
}
