package store

import (
	"testing"
	"time"
)

func TestRunLifecycle(t *testing.T) {
	p := openTmpProject(t)
	if err := p.PutRun(Run{ID: "run-1", Agent: "bombur", Command: "go test ./..."}); err != nil {
		t.Fatal(err)
	}
	r, ok, err := p.GetRun("run-1")
	if err != nil || !ok {
		t.Fatalf("get run: ok=%v err=%v", ok, err)
	}
	if r.Status != "queued" || r.Agent != "bombur" || r.Project != "repo" || r.Command != "go test ./..." {
		t.Fatalf("run defaults wrong: %+v", r)
	}
	firstUpdate, err := time.Parse(time.RFC3339, r.UpdatedAt)
	if err != nil || time.Since(firstUpdate) > time.Minute {
		t.Fatalf("updated_at not stamped on insert: %q (err %v)", r.UpdatedAt, err)
	}

	// Status filter.
	if err := p.PutRun(Run{ID: "run-2", Agent: "bombur", Status: "passed"}); err != nil {
		t.Fatal(err)
	}
	queued, _ := p.Runs("queued")
	if len(queued) != 1 || queued[0].ID != "run-1" {
		t.Fatalf("queued filter wrong: %+v", queued)
	}
	all, _ := p.Runs()
	if len(all) != 2 {
		t.Fatalf("all runs: %d", len(all))
	}
	if g, _ := p.s.AllRuns(); len(g) != 2 || g[0].Project != "repo" {
		t.Fatalf("AllRuns: %+v", g)
	}

	// Priority, then status transitions.
	if err := p.SetRunPriority("run-1", "P0"); err != nil {
		t.Fatal(err)
	}
	got, _, _ := p.GetRun("run-1")
	if got.Priority != "P0" {
		t.Fatalf("priority not persisted: %+v", got)
	}
	secondUpdate, err := time.Parse(time.RFC3339, got.UpdatedAt)
	if err != nil || secondUpdate.Before(firstUpdate) {
		t.Fatalf("updated_at not refreshed on priority write: first=%v second=%q (err %v)", firstUpdate, got.UpdatedAt, err)
	}
	if got.FinishedAt != "" {
		t.Fatalf("a queued run must have no finished_at: %+v", got)
	}

	if err := p.SetRunStatus("run-1", "running"); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := p.GetRun("run-1"); got.Status != "running" || got.FinishedAt != "" {
		t.Fatalf("running must not stamp finished_at: %+v", got)
	}
	if err := p.SetRunStatus("run-1", "failed"); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := p.GetRun("run-1"); got.Status != "failed" || got.FinishedAt == "" {
		t.Fatalf("a terminal status must stamp finished_at: %+v", got)
	}
}

func TestRunOutput(t *testing.T) {
	p := openTmpProject(t)
	if err := p.PutRun(Run{ID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	if out, err := p.RunOutput("run-1"); err != nil || out != "" {
		t.Fatalf("output before anything writes it = %q, err=%v, want empty", out, err)
	}
	if out, err := p.RunOutput("no-such-run"); err != nil || out != "" {
		t.Fatalf("output for an unknown run = %q, err=%v, want empty and no error", out, err)
	}
}
