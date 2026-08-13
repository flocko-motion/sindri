// package: hub/workflow / execrun
// type:    logic (execute one queued run)
// job:     run a single queued run to completion: a fresh, memory-capped container from
// the project's own image, a materialized (not live) copy of the agent's worktree,
// a persistent build cache, and a hard 15-minute cap — then record the outcome and
// tell the scheduling agent a summary, never the full log.
// limits:  one run, synchronously, start to finish. Picking which run goes next
// (-> NextQueuedRun) and enforcing only one runs at a time (-> hub/runwatch.go)
// are the caller's.
package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
// on record. A returned error means the run was never attempted at all — its own outcome (a
// failing test, a timeout) is recorded, not returned.
func (e *Engine) ExecuteRun(project, id string) error {
	ps := e.store.For(project)
	r, ok, err := ps.GetRun(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such run %q", id)
	}
	root := e.deps.ProjectRoot(project)
	a, ok, err := ps.GetAgent(r.Agent)
	if err != nil {
		return err
	}
	if !ok {
		return e.finishRun(ps, project, r, "failed", fmt.Sprintf("run: agent %s no longer exists\n", r.Agent), 0, 0)
	}

	mwt, err := repo.MaterializeRun(root, filepath.Join(root, a.Workspace), id)
	if err != nil {
		return e.finishRun(ps, project, r, "failed", fmt.Sprintf("run: could not prepare a workspace: %s\n", err), 0, 0)
	}
	defer func() { _ = repo.RemoveRunMaterialization(mwt) }()

	cfg, err := e.deps.ProjectConfig(project)
	if err != nil {
		return e.finishRun(ps, project, r, "failed", fmt.Sprintf("run: could not read project config: %s\n", err), 0, 0)
	}
	imageRef, err := container.EnsureImage(root, config.Abs(root, cfg.Containerfile), io.Discard)
	if err != nil {
		return e.finishRun(ps, project, r, "failed", fmt.Sprintf("run: could not prepare the image: %s\n", err), 0, 0)
	}
	cacheMounts, cacheEnv, err := runCacheMounts(project)
	if err != nil {
		return e.finishRun(ps, project, r, "failed", fmt.Sprintf("run: could not prepare the build cache: %s\n", err), 0, 0)
	}

	if err := ps.SetRunStatus(id, "running"); err != nil {
		return err
	}
	e.deps.Notify()

	name := e.deps.Container(project, "run-"+id)
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
		return e.finishRun(ps, project, r, "failed", fmt.Sprintf("run: could not start a container: %s\n", err), 0, 0)
	}
	defer func() { _ = container.Rm(name) }()

	budget := runTimeout(r.Timeout)
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	start := time.Now()
	out, execErr := container.ExecContext(ctx, name, "sh", "-c", r.Command)
	elapsed := time.Since(start)

	status := "passed"
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		status = "timed_out"
	case execErr != nil:
		status = "failed"
	}
	output := string(out)
	if status == "timed_out" {
		output += fmt.Sprintf("\nrun: exceeded its %s budget and was stopped — the container is gone; nothing from it keeps running.\n", budget)
	}
	return e.finishRun(ps, project, r, status, output, elapsed, budget)
}

// finishRun records a run's outcome (full, uncapped output; terminal status) and injects a
// summary into the scheduling agent's session — never the full log, which is what turns a large
// run into an oversized session (-> MsgRunFinished).
func (e *Engine) finishRun(ps *store.ProjectStore, project string, r api.Run, status, output string, elapsed, budget time.Duration) error {
	if err := ps.SetRunOutput(r.ID, output); err != nil {
		return err
	}
	if err := ps.SetRunStatus(r.ID, status); err != nil {
		return err
	}
	e.deps.Notify()
	_ = e.deps.InjectWhenReady(project, r.Agent, MsgRunFinished(r.ID, status, elapsed, budget))
	return nil
}

// runCacheMounts returns a run's persistent build-cache mounts and the env vars pointing Go at
// them, creating the host directories on demand — without one, a cold container turns a
// 30-second suite into a multi-minute rebuild every run. Safe to share across runs unguarded:
// with one run at a time fleet-wide, there is never a second writer.
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
