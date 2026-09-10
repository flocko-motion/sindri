// package: hub/flow/agent / roles
// type:    assembly (the four flows, collected)
// job:     hand the machine one registry over the four role flows, and answer which flow a role
// runs — the only place that knows all four exist.
// limits:  the registry and the acting half. Every file HERE acts — the maps are one level down,
// one package per role (worker/, planner/, reviewer/, coauthor/), which is why nothing at this
// level carries the _act suffix the single-directory subjects need (-> internal/arch/flow_test.go).
package agent

import (
	"fmt"

	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/coauthor"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/planner"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/reviewer"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker"
)

// Roles are the four an agent can have. Closed: a role outside this set has no flow, and an agent
// with no flow is one nobody can answer for.
var Roles = []string{"worker", "planner", "reviewer", "coauthor"}

// flows is each role's own map. Nothing here is shared between them — a clear leads somewhere
// different for a worker than for a reviewer, so each declares its own rather than jumping back to
// a remembered address.
var flows = map[string][]flow.State{
	"worker":   worker.Flow,
	"planner":  planner.Flow,
	"reviewer": reviewer.Flow,
	"coauthor": coauthor.Flow,
}

// starts is where an agent of each role begins when nothing is stored for it.
var starts = map[string]string{
	"worker":   worker.Start,
	"planner":  planner.Start,
	"reviewer": reviewer.Start,
	"coauthor": coauthor.Start,
}

// Of is one role's flow — what "the planner's flow" means as a value rather than as a way of
// reading the code.
func Of(role string) []flow.State { return flows[role] }

// Start is where an agent of this role begins.
func Start(role string) (string, error) {
	s, ok := starts[role]
	if !ok {
		return "", fmt.Errorf("flow: no flow is declared for role %q", role)
	}
	return s, nil
}

// All is every state of every role, which is what the machine is registered over: the subject is an
// AGENT and not a role, a role can change under an agent, and the engine's one-action-per-subject
// guarantee is about that agent — something four machines could not promise between them.
var All = func() []flow.State {
	var out []flow.State
	for _, role := range Roles {
		out = append(out, flows[role]...)
	}
	return out
}()

// StartFor is the start state for an agent of this role, for the machine's own Start. Every role
// starts somewhere, so a caller that reaches an unknown one is told rather than defaulted.
func StartFor(role string) string {
	if s, err := Start(role); err == nil {
		return s
	}
	return worker.Start
}
