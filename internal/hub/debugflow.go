// package: hub / debugflow
// type:    assembly (the flow debug view, wired to the hub)
// job:     answer the debug view's reads by subject kind (its listener starts with the hub: serve.go).
// limits:  dispatch. The graph and the reading are each kind's engine's (-> fleet.AgentGraph); the
// listener and its read-only surface are debugview's.
package hub

import (
	"fmt"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/flow/fleet"
)

// Graph is one flow's declarations, by kind. Only agents have a view yet; PRs, tasks and runs are
// declared the same way and join by adding a case here and in Subject.
func (h *Hub) Graph(kind, variant string) (api.FlowGraph, error) {
	switch kind {
	case "agent":
		return fleet.AgentGraph(variant)
	}
	return api.FlowGraph{}, fmt.Errorf("the debug view has no flow of kind %q", kind)
}

// Subject reads one subject against its flow, by kind.
func (h *Hub) Subject(kind, project, id string) (api.SubjectFlow, error) {
	switch kind {
	case "agent":
		return h.wf.AgentSubject(project, id)
	}
	return api.SubjectFlow{}, fmt.Errorf("the debug view has no subject of kind %q", kind)
}
