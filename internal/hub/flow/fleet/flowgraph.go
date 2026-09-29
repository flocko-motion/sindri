// package: hub/flow/fleet / flowgraph
// type:    assembly (an agent's flow, and its place in it, as data)
// job:     the agent kind of the flow debug view: a role's graph, and one agent read against it —
// its state, where a pass would settle it, which exits hold now, the observer's reading, and its
// history with each recorded move resolved to its edge.
// limits:  reading. The export is the machine's (-> machine.Graph, machine.Weigh); nothing here
// moves or starts anything.
package fleet

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
	agentflow "github.com/flo-at/sindri/internal/hub/flow/agent"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
)

// historyLimit is how many state-log rows the view shows; the store caps what it keeps anyway.
const historyLimit = 300

// AgentGraph is one role's declared flow.
func AgentGraph(role string) (api.FlowGraph, error) {
	start, err := agentflow.Start(role)
	if err != nil {
		return api.FlowGraph{}, err
	}
	file, pkg := agentflow.LayoutFile(role)
	pos := make(map[string]api.FlowPos, len(agentflow.LayoutOf(role)))
	for name, p := range agentflow.LayoutOf(role) {
		pos[name] = api.FlowPos{X: p.X, Y: p.Y}
	}
	states, groups := machine.Graph(agentflow.Of(role), agentflow.GroupsOf(role))
	return api.FlowGraph{Kind: "agent", Variant: role, Start: start, States: states, Groups: groups,
		Positions: pos, LayoutFile: file, LayoutPackage: pkg}, nil
}

// AgentSubject reads one agent against its flow, on a world gathered now as a pass would gather it.
func (e *Engine) AgentSubject(project, name string) (api.SubjectFlow, error) {
	ps := e.Store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return api.SubjectFlow{}, err
	}
	if !ok {
		return api.SubjectFlow{}, fmt.Errorf("no agent %q in project %s", name, project)
	}
	if e.flow == nil {
		return api.SubjectFlow{}, fmt.Errorf("the agent flow machine is not running")
	}
	subject := project + "/" + name
	s, holds, err := e.flow.Weigh(subject)
	if err != nil {
		return api.SubjectFlow{}, err
	}
	would, err := e.flow.Would(subject)
	if err != nil {
		return api.SubjectFlow{}, err
	}
	rows, err := ps.StateLog(name, historyLimit)
	if err != nil {
		return api.SubjectFlow{}, err
	}
	hist := make([]api.FlowHistory, 0, len(rows))
	for _, r := range rows {
		h := api.FlowHistory{StateEvent: r}
		if r.Reason == string(machine.StepMoved) {
			h.Move = parseMove(r.Detail)
		}
		hist = append(hist, h)
	}
	return api.SubjectFlow{Kind: "agent", Project: project, ID: name, Variant: a.Role, State: s.Name,
		Would: would.Name, Holds: holds, Facts: e.agentFacts(project, name), History: hist}, nil
}

// agentFacts is the observer's standing reading, which every pod condition was gathered from.
func (e *Engine) agentFacts(project, name string) []api.FlowFact {
	o := e.Harness.Observe(project, name)
	seen := "never"
	if o.Seen() {
		seen = o.TakenAt.UTC().Format(time.RFC3339)
	}
	b := strconv.FormatBool
	return []api.FlowFact{
		{Name: "up", Value: b(o.Up)}, {Name: "session", Value: o.State.String()},
		{Name: "seen", Value: seen}, {Name: "clients", Value: strconv.Itoa(o.Clients)},
		{Name: "launching", Value: b(o.Launching)}, {Name: "launch failed", Value: b(o.LaunchFailed)},
		{Name: "stopping", Value: b(o.Stopping)},
	}
}

// parseMove reads the edge out of a recorded move, "<from>: <event> -> <to>: <why>": the recorder
// prefixes the state (-> passRecorder) to the machine's own "<event> -> <to>: <why>". Nil when the
// row does not have that shape, so a view never draws an edge it guessed.
func parseMove(detail string) *api.FlowMove {
	from, rest, ok := strings.Cut(detail, ": ")
	if !ok {
		return nil
	}
	on, rest, ok := strings.Cut(rest, " -> ")
	if !ok {
		return nil
	}
	to, _, ok := strings.Cut(rest, ": ")
	if !ok || from == "" || on == "" || to == "" {
		return nil
	}
	return &api.FlowMove{From: from, On: on, To: to}
}
