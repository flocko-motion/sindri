package agent

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/store"
)

// clearTestDeps is a minimal agent.Deps: enough for ClearContext's refusal paths, which never
// reach Rehydrate/RefreshTask/ProjectConfig.
type clearTestDeps struct{}

func (clearTestDeps) Notify()                                     {}
func (clearTestDeps) ContainerName(_, name string) string         { return "no-such-container-" + name }
func (clearTestDeps) ProjectRoot(string) string                   { return "" }
func (clearTestDeps) ProjectConfig(string) (config.Config, error) { return config.Config{}, nil }
func (clearTestDeps) ArchitectureDoc(string) string               { return "" }
func (clearTestDeps) RefreshTask(_, _ string) error               { return nil }
func (clearTestDeps) Rehydrate(_, _ string)                       {}

// TestClearContextRefusesMidTask is the guard the whole feature rests on: an agent holding a
// leaf task or a container has file-tree memory /clear would silently invalidate (claimLeaf only
// resets the worktree on its OWN next claim, not this one), so clearing must refuse rather than
// run and leave the agent confidently wrong about what's on disk.
func TestClearContextRefusesMidTask(t *testing.T) {
	_, st := newService(t)
	s := New(st, clearTestDeps{}, nil)
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		st   store.AgentState
	}{
		{"leaf task held", store.AgentState{Agent: "eitri", Task: "td-abc123", Phase: "working"}},
		{"container held", store.AgentState{Agent: "eitri", Container: "td-epic", Phase: "working"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ps.SetState(tc.st); err != nil {
				t.Fatal(err)
			}
			err := s.ClearContext("proj", "eitri")
			if err == nil {
				t.Fatal("ClearContext succeeded while the agent still held work — it must refuse")
			}
			if !strings.Contains(err.Error(), "leaf boundary") {
				t.Errorf("error = %q, want it to explain the leaf-boundary rule", err.Error())
			}
		})
	}
}

// TestClearContextRefusesADownAgent: an idle agent with no held task but no live pod either has
// nothing to send /clear into.
func TestClearContextRefusesADownAgent(t *testing.T) {
	_, st := newService(t)
	s := New(st, clearTestDeps{}, nil)
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearContext("proj", "eitri"); err == nil {
		t.Fatal("ClearContext succeeded against an agent with no running container")
	}
}
