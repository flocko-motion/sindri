// package: hub/flowtest / place
// type:    assembly (the one door a fixture puts an agent through)
// job:     put an agent where a test needs it — what it HOLDS and where it STANDS — through the two
// writers the hub itself uses, and refuse a state no role's flow declares.
// limits:  placing. Whether a fixture SHOULD be able to reach a state is the flow's answer, and this
// asks it rather than deciding.
package flowtest

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/coauthor"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/planner"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/reviewer"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// Place puts an agent where a fixture needs it, through the two writers the hub uses: what it holds,
// then where it stands. The state may be the bare word fixtures have always used ("working"),
// resolved against the agent's own role — and a word no role declares FAILS the test, since an agent
// standing somewhere undeclared reads as standing at its role's start.
func Place(t *testing.T, ps *store.ProjectStore, st store.AgentState) {
	t.Helper()
	if err := ps.SetHolding(st.Agent, st.Task, st.Branch, st.Container, store.ReasonClaimed, "fixture"); err != nil {
		t.Fatalf("placing what %s holds: %v", st.Agent, err)
	}
	if st.Phase == "" {
		return // what it holds, and wherever its own flow begins
	}
	// An agent on no roster is placed as a worker, which is the same fallback the machine makes when
	// it cannot name a role (-> agent.StartFor): a fixture about something other than roles need not
	// grow a roster row to say where its agent stands.
	role := "worker"
	if ag, ok, err := ps.GetAgent(st.Agent); err == nil && ok && ag.Role != "" {
		role = ag.Role
	}
	state := Declared(t, role, st.Phase)
	if err := ps.SetPhase(st.Agent, state, store.ReasonAdvanced, "fixture"); err != nil {
		t.Fatalf("placing %s in %s: %v", st.Agent, state, err)
	}
}

// Declared resolves a phase word to the state it names for one role, failing the test when it names
// none. A bare word is prefixed with the role, so "working" and "worker/working" both reach the one
// state — and a word neither form declares is a fixture reaching for somewhere that does not exist.
func Declared(t *testing.T, role, phase string) string {
	t.Helper()
	state := phase
	if !strings.Contains(phase, "/") {
		state = role + "/" + phase
	}
	for _, s := range flowOf(role) {
		if s.Name == state {
			return state
		}
	}
	t.Fatalf("a %s has no state %q — a fixture may only build a world the machine could produce", role, state)
	return ""
}

// flowOf is one role's declared states. The role packages directly rather than their aggregator,
// which cannot be imported here: the aggregator's own tests import this package.
func flowOf(role string) []flow.State {
	switch role {
	case "reviewer":
		return reviewer.Flow
	case "planner":
		return planner.Flow
	case "coauthor":
		return coauthor.Flow
	}
	return worker.Flow
}
