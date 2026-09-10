package agent

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/store"
)

// proj is the one repo these fixtures register, named as the fleet's tests have always named it.
const proj = flowtest.TestProject

// newActWith is an agent's acting half over a real store and the fake hub every subject shares.
func newActWith(t *testing.T, d *flowtest.Hub) (*Act, *store.ProjectStore, *flowtest.Hub) {
	t.Helper()
	if d == nil {
		d = &flowtest.Hub{}
	}
	c, ps := flowtest.Core(t, d)
	return New(c), ps, d
}

// newActOn is newActWith for a fixture that opened its own store and seeded it first.
func newActOn(t *testing.T, st *store.Store, d *flowtest.Hub) *Act {
	t.Helper()
	if d.Root == "" {
		d.Root = t.TempDir()
	}
	return New(flowtest.Over(st, d))
}
