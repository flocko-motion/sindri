// package: hub/flowtest / roster
// type:    assembly (the roster-row facts a human sets)
// job:     write the flags a human's own verbs write — retirement, for now — so a fixture states a
// decision rather than reaching for the service that carries it out.
// limits:  the flags. What each one MEANS is the surface's (-> situation.Surface), and no fixture
// here reads one to decide anything.
package flowtest

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// Retire sets an agent's retirement flag, which is what the hub's own retire verb writes.
func Retire(t *testing.T, ps *store.ProjectStore, name string) {
	t.Helper()
	ag, _, _ := ps.GetAgent(name)
	ag.Retired = true
	if err := ps.PutAgent(ag); err != nil {
		t.Fatal(err)
	}
}
