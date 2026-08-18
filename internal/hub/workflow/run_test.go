package workflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

func TestQueuePositions(t *testing.T) {
	runs := []api.Run{
		{ID: "low", Status: "queued", Priority: "", CreatedAt: "2026-01-01T00:00:00Z"},
		{ID: "p1-first", Status: "queued", Priority: "P1", CreatedAt: "2026-01-01T00:00:00Z"},
		{ID: "p1-second", Status: "queued", Priority: "P1", CreatedAt: "2026-01-01T00:00:01Z"},
		{ID: "p0", Status: "queued", Priority: "P0", CreatedAt: "2026-01-01T00:00:02Z"},
		{ID: "running", Status: "running"},
		{ID: "done", Status: "passed"},
	}
	pos := queuePositions(runs)
	want := map[string]int{"p0": 1, "p1-first": 2, "p1-second": 3, "low": 4}
	for id, w := range want {
		if pos[id] != w {
			t.Errorf("position[%q] = %d, want %d (got %v)", id, pos[id], w, pos)
		}
	}
	for _, id := range []string{"running", "done"} {
		if _, ok := pos[id]; ok {
			t.Errorf("%q is not queued and must have no position", id)
		}
	}
}

func TestCapRunOutput(t *testing.T) {
	if got := capRunOutput(""); got != "" {
		t.Errorf("capRunOutput(\"\") = %q, want empty", got)
	}
	short := "line1\nline2\n"
	if got := capRunOutput(short); got != short {
		t.Errorf("short output must pass through unchanged: got %q, want %q", got, short)
	}
	long := ""
	for i := 0; i < runOutputCap+50; i++ {
		long += "line\n"
	}
	got := capRunOutput(long)
	if got == long {
		t.Fatal("output over the cap must be trimmed")
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("a trimmed output must say so, never silently: %q", got[:60])
	}
}

// runEngine is a store + engine with one registered project, for the run workflow methods.
func runEngine(t *testing.T) (*Engine, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	deps := &stubDeps{root: root, projects: []store.Project{{Tag: "repo", Path: root}}}
	return New(st, deps), ps
}

func TestScheduleRunThenFleetRunsRanksIt(t *testing.T) {
	e, _ := runEngine(t)
	r, err := e.ScheduleRun("repo", "bombur", "go test ./...", "", "")
	if err != nil {
		t.Fatalf("ScheduleRun: %v", err)
	}
	if r.Status != "queued" || r.Agent != "bombur" || r.Command != "go test ./..." {
		t.Fatalf("scheduled run wrong: %+v", r)
	}
	fleet, err := e.FleetRuns()
	if err != nil {
		t.Fatalf("FleetRuns: %v", err)
	}
	if len(fleet) != 1 || fleet[0].ID != r.ID {
		t.Fatalf("FleetRuns = %+v, want just %q", fleet, r.ID)
	}
	if fleet[0].Position != 1 {
		t.Errorf("the only queued run must be position 1, got %d", fleet[0].Position)
	}
}

func TestRunInfoCapsOutputAndReportsPosition(t *testing.T) {
	e, ps := runEngine(t)
	r, err := e.ScheduleRun("repo", "bombur", "go test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ScheduleRun("repo", "nori", "go build", "P0", ""); err != nil { // ranked ahead of r
		t.Fatal(err)
	}
	d, err := e.RunInfo("repo", r.ID)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	if d.Run.Position != 2 {
		t.Errorf("position with a P0 run ahead of it = %d, want 2", d.Run.Position)
	}
	if d.Output != "" {
		t.Errorf("nothing has written output yet: got %q", d.Output)
	}
	_ = ps
}

func TestCancelRun(t *testing.T) {
	e, ps := runEngine(t)
	r, err := e.ScheduleRun("repo", "bombur", "go test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.CancelRun("repo", r.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	got, _, _ := ps.GetRun(r.ID)
	if got.Status != "cancelled" || got.FinishedAt == "" {
		t.Fatalf("cancelled run = %+v, want status cancelled with finished_at set", got)
	}
	// Already settled: a second cancel must be refused, not silently re-applied.
	if err := e.CancelRun("repo", r.ID); err == nil {
		t.Error("cancelling an already-finished run must be refused")
	}
	if err := e.CancelRun("repo", "no-such-run"); err == nil {
		t.Error("cancelling an unknown run must error")
	}
}

func TestReprioritiseRun(t *testing.T) {
	e, ps := runEngine(t)
	r, err := e.ScheduleRun("repo", "bombur", "go test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.ReprioritiseRun("repo", r.ID, "P0"); err != nil {
		t.Fatalf("reprioritise: %v", err)
	}
	if got, _, _ := ps.GetRun(r.ID); got.Priority != "P0" {
		t.Fatalf("priority not persisted: %+v", got)
	}
	if err := ps.SetRunStatus(r.ID, "running"); err != nil {
		t.Fatal(err)
	}
	// No longer queued: reprioritising a running (or finished) run has nothing left to reorder.
	if err := e.ReprioritiseRun("repo", r.ID, "P1"); err == nil {
		t.Error("reprioritising a running run must be refused")
	}
}

func TestRunProjectFindsTheOwner(t *testing.T) {
	e, _ := runEngine(t)
	r, err := e.ScheduleRun("repo", "bombur", "go test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := e.RunProject("elsewhere", r.ID); got != "repo" {
		t.Errorf("RunProject(%q) = %q, want repo", r.ID, got)
	}
	if got := e.RunProject("repo", "no-such-run"); got != "repo" {
		t.Errorf("an unknown id should fall back to the caller's own project, got %q", got)
	}
}

// TestCmdScheduleRun is the agent-facing verb sd-68f8e7 adds: it queues at once and reports the
// run's position, rather than blocking — the same act-report-idle contract as submit.
func TestCmdScheduleRun(t *testing.T) {
	e, ps := runEngine(t)
	c := registry.Caller{Project: "repo", Agent: "bombur", Role: "worker"}

	var out strings.Builder
	if code, err := e.CmdScheduleRun(c, nil, &out); err != nil || code != 2 {
		t.Fatalf("empty command should be a usage error: code=%d err=%v", code, err)
	}

	out.Reset()
	if code, err := e.CmdScheduleRun(c, []string{"go", "test", "./..."}, &out); err != nil || code != 0 {
		t.Fatalf("CmdScheduleRun: code=%d err=%v", code, err)
	}
	if !strings.Contains(out.String(), "position 1") {
		t.Errorf("reply should report a queue position: %q", out.String())
	}
	if !strings.Contains(out.String(), "run-") {
		t.Errorf("reply should name the run id: %q", out.String())
	}

	runs, err := ps.Runs()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Command != "go test ./..." || runs[0].Agent != "bombur" {
		t.Fatalf("scheduled run wrong: %+v", runs)
	}
}
