// package: hub/agent / model_fixture_test
// type:    logic (a live agent with a transcript on disk)
// job:     seed a running agent whose session can be measured — a pane to inject into and a real
// transcript — for the tests that ask what a model switch does to the reading.
// limits:  seeding only.
package agent

import (
	"testing"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/agent/claude"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
)

// modelFixture wires a service over a fake tmux backend and the real transcript reader (so
// CompactionThreshold computes against an actual file, not a stub), with one agent registered and
// idle. Deciding WHEN to compact is fleet.Engine's (task/review awareness Compact itself does
// not have); this only exercises Compact's own mechanics.
func modelFixture(t *testing.T) (*Service, *fakeRuntime) {
	t.Helper()
	t.Setenv("SINDRI_HOME", t.TempDir()) // AgentHomeDir reads this, so the transcript is ours
	_, st := newService(t)
	s := New(st, tellDeps{}, nil)
	if err := st.For("proj").PutAgent(store.Agent{Name: "durin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := st.For("proj").SetState(store.AgentState{Agent: "durin", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	f := &fakeRuntime{pane: idlePane}
	container.Use(f)
	agentport.Use(claude.New())
	t.Cleanup(func() {
		container.UseDefault()
		agentport.Use(unreadablePane{})
	})
	s.ForgetContext("proj", "durin") // contextMemo is package-level; a prior test's reading must not leak in
	return s, f
}
