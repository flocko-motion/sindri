package verbs

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/flow/pr"
	"github.com/flo-at/sindri/internal/hub/flow/run"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/store"
)

// proj is the one repo these fixtures register, named as the fleet's tests have always named it.
const proj = flowtest.TestProject

// newActOn is newActWith for a fixture that opened its own store and seeded it first.
func newActOn(t *testing.T, st *store.Store, d *flowtest.Hub) *Act {
	t.Helper()
	if d.Root == "" {
		d.Root = t.TempDir()
	}
	c := flowtest.Over(st, d)
	c.Gate = pr.New(c)
	return New(c)
}

// runQueuedGate takes the gate a contribute just queued and runs it, so a test can assert what a
// PASSING gate unlocks rather than only that something was queued.
func runQueuedGate(t *testing.T, a *Act) {
	t.Helper()
	project, id, ok := run.New(a.Core).NextQueuedRun()
	if !ok {
		t.Fatal("expected a queued gate run")
	}
	if err := run.New(a.Core).ExecuteRun(t.Context(), project, id); err != nil {
		t.Fatalf("ExecuteRun(%s): %v", id, err)
	}
}
