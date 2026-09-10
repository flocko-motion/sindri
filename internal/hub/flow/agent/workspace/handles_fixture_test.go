package workspace

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/flow/pr"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/world/store"
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
