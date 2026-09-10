// package: hub/flow/fleet / flowmachine
// type:    assembly (the flow machine, wired to the hub)
// job:     hand the engine the four role flows plus everything they need from the hub — how a
// world is gathered, where a state is stored, what each action does, and where the record goes.
// limits:  the wiring. The maps are hub/flow/roles', the engine is hub/flow/machine's.
package fleet

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	agentflow "github.com/flo-at/sindri/internal/hub/flow/agent"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
	flowpr "github.com/flo-at/sindri/internal/hub/flow/pr"
	runflow "github.com/flo-at/sindri/internal/hub/flow/run"
	flowtask "github.com/flo-at/sindri/internal/hub/flow/task"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// Beat is how often the engine looks for agents whose own cadence has come due. Short against every
// declared tolerance, so what is felt is the cadence a condition asked for rather than this.
const Beat = 2 * time.Second

// Reconciling turns the background loops on for every subject the hub owns — agents, tasks and merge
// intents — so conditions are watched and actions run on their own beat rather than only when
// something asks. An engine without it answers from exactly the same maps, on demand.
func (e *Engine) Reconciling() *Engine {
	if m, err := e.newFlow(e.Lifetime, Beat); err != nil {
		log.Printf("hub: the agent flow would not start: %v", err)
	} else {
		closeMachine(e.flow)
		e.flow = m
	}
	if m, err := e.newTaskFlow(e.Lifetime, Beat); err != nil {
		log.Printf("hub: the task flow would not start: %v", err)
	} else {
		closeTaskMachine(e.tasks)
		e.tasks = m
	}
	if m, err := e.newPRFlow(e.Lifetime, Beat); err != nil {
		log.Printf("hub: the pull-request flow would not start: %v", err)
	} else {
		closePRMachine(e.prs)
		e.prs = m
	}
	if m, err := e.runAct().NewRunFlow(e.Lifetime, Beat); err != nil {
		log.Printf("hub: the run flow would not start: %v", err)
	} else {
		closeRunMachine(e.runs)
		e.runs = m
	}
	return e
}

// Close stops every flow the engine runs, and the actions they have in flight.
func (e *Engine) Close() {
	closeMachine(e.flow)
	closeTaskMachine(e.tasks)
	closePRMachine(e.prs)
	closeRunMachine(e.runs)
}

// WakeRuns tells every unsettled run that something happened — the fleet's one slot freeing, above
// all. Without it a queue waits out its poll to notice the slot it could have taken at once.
func (e *Engine) WakeRuns(t machine.Topic) {
	if e.runs == nil {
		return
	}
	for _, s := range e.runAct().OpenRuns() {
		e.runs.Wake(s, t)
	}
}

// newFlow builds the machine over the four role flows. With no tick it is ON DEMAND: it answers
// where an agent stands and runs a pass when asked, and nothing happens on its own. The hub turns
// the loop on (-> Reconciling).
func (e *Engine) newFlow(lifetime context.Context, beat time.Duration) (machine.Machine[flow.World], error) {
	return machine.New(lifetime, machine.Config[flow.World]{
		States:   agentflow.All,
		Start:    agentflow.StartFor("worker"),
		Gather:   e.gatherSubject,
		Stored:   e.storedState,
		Move:     e.moveState,
		Do:       e.doers(),
		Subjects: e.subjects,
		Default:  flow.DefaultEvery,
		Tick:     beat,
		Record:   passRecorder{e},
	})
}

// storedState reads where an agent stands. An agent with nothing stored starts where its own role's
// flow begins — the state is per agent, so a role change moves it to that role's start.
func (e *Engine) storedState(s string) (string, time.Time, error) {
	project, agent, err := subject(s)
	if err != nil {
		return "", time.Time{}, err
	}
	ps := e.Store.For(project)
	st, err := ps.GetState(agent)
	if err != nil {
		return "", time.Time{}, err
	}
	a, ok, err := ps.GetAgent(agent)
	if err != nil || !ok {
		return "", time.Time{}, err
	}
	since, _ := time.Parse(time.RFC3339, st.PhaseSince)
	if strings.HasPrefix(st.Phase, a.Role+"/") {
		return st.Phase, since, nil
	}
	if s := legacy(a.Role, st.Phase); s != "" {
		return s, since, nil // a phase written before states were named; the agent has not moved
	}
	// Nothing stored, or a state belonging to a role this agent no longer has.
	return agentflow.StartFor(a.Role), since, nil
}

// legacy maps a phase word written before states carried their role onto the state it means, "" when
// there is none. A hub upgrading in place has rows saying "working", and an agent must not be swept
// back to the start of its flow just because the vocabulary grew a prefix.
func legacy(role, phase string) string {
	named := map[string]string{
		"idle": "idle", "working": "working", "submitted": "submitted",
		"gating": "gating", "resolving": "resolving", "reviewing": "reviewing",
		"planning": "planning", "collab": "collab",
	}[phase]
	if named == "" {
		return ""
	}
	for _, s := range agentflow.Of(role) {
		if s.Name == role+"/"+named {
			return s.Name
		}
	}
	return ""
}

// moveState writes an agent's new state, with the words the transition carried.
func (e *Engine) moveState(s, from, to, why string) error {
	project, agent, err := subject(s)
	if err != nil {
		return err
	}
	ps := e.Store.For(project)
	// Entering a resting state RELEASES the work. SetPhase alone preserves the row, which left an
	// agent idle with a task still on it — free on the board and refused by the assigner at once.
	if flow.Releases(to) {
		err := ps.SetState(store.AgentState{Agent: agent, Phase: to}, store.ReasonFreed, from+" -> "+to+": "+why)
		e.Deps.Notify()
		return err
	}
	if perr := ps.SetPhase(agent, to, store.ReasonAdvanced, from+" -> "+to+": "+why); perr == nil {
		e.Deps.Notify()
		return nil
	}
	st, _ := ps.GetState(agent)
	err = ps.SetState(store.AgentState{Agent: agent, Task: st.Task, Branch: st.Branch,
		Container: st.Container, Phase: to}, store.ReasonAdvanced, from+" -> "+to+": "+why)
	e.Deps.Notify()
	return err
}

// subjects is every agent the hub knows, as "project/agent".
func (e *Engine) subjects() []string {
	projects, err := e.Store.Projects()
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range projects {
		roster, rerr := e.Store.For(p.Tag).Roster()
		if rerr != nil {
			continue
		}
		for _, a := range roster {
			out = append(out, p.Tag+"/"+a.Name)
		}
	}
	return out
}

// Wake tells the machine that topic happened, so any agent watching for it looks now rather than at
// its own next beat. A HINT: a dropped one costs at most that state's cadence, never correctness.
func (e *Engine) Wake(project, agent string, topic machine.Topic) {
	if e.flow != nil {
		e.flow.Wake(project+"/"+agent, topic)
	}
}

// WakeProject tells every agent in a project. What a task event means: the task nobody named could
// be the next one for any of them.
func (e *Engine) WakeProject(project string, topic machine.Topic) {
	if e.flow == nil {
		return
	}
	roster, err := e.Store.For(project).Roster()
	if err != nil {
		return
	}
	for _, a := range roster {
		e.flow.Wake(project+"/"+a.Name, topic)
	}
}

// WakeAll tells every agent the hub knows. What the board is told, the decider is told.
func (e *Engine) WakeAll(topic machine.Topic) {
	if e.flow == nil {
		return
	}
	for _, s := range e.subjects() {
		e.flow.Wake(s, topic)
	}
}

// FlowOf renders one role's map — what the hub answers when asked how that kind of agent works.
func FlowOf(role string) string { return machine.Table(agentflow.Of(role)) }

// passRecorder keeps the machine's account of itself on the agent's own durable record, under the
// correlation id that pass carries — so "what happened to this agent" is one query.
type passRecorder struct{ e *Engine }

func (r passRecorder) Record(en machine.Entry) {
	project, agent, err := subject(en.Subject)
	if err != nil {
		return
	}
	detail := en.Detail
	if en.State != "" {
		detail = en.State + ": " + detail
	}
	if en.Err != nil {
		detail += " — " + en.Err.Error()
	}
	if lerr := r.e.Store.For(project).LogPass(agent, string(en.Step), en.Pass, detail); lerr != nil {
		log.Printf("hub: %v", lerr)
	}
}

// Look runs one pass over an agent now rather than at its own next beat — for a caller that wants
// the world reconciled before it goes on, and for a test, which cannot wait on a beat.
func (e *Engine) Look(project, agent string) {
	if e.flow != nil {
		e.flow.Look(project + "/" + agent)
	}
}

// LookProject runs one pass over every agent in a project.
func (e *Engine) LookProject(project string) {
	roster, err := e.Store.For(project).Roster()
	if err != nil {
		return
	}
	for _, a := range roster {
		e.Look(project, a.Name)
	}
}

// The three closers exist only because Go has no way to say "any machine" without a type parameter,
// and a nil one is the ordinary case for an engine that never turned its loops on.
func closeMachine(m machine.Machine[flow.World]) {
	if m != nil {
		_ = m.Close()
	}
}

func closeTaskMachine(m machine.Machine[flowtask.World]) {
	if m != nil {
		_ = m.Close()
	}
}

func closePRMachine(m machine.Machine[flowpr.World]) {
	if m != nil {
		_ = m.Close()
	}
}

func closeRunMachine(m machine.Machine[runflow.World]) {
	if m != nil {
		_ = m.Close()
	}
}

// LookRun settles one queued run now rather than at the machine's next beat — for a caller that has
// just changed something under it, and for a test, which cannot wait on a beat.
func (e *Engine) LookRun(project, id string) {
	if e.runs != nil {
		e.runs.Look(project + "/" + id)
	}
}

// standingBecause is why a state holds verbs back, in its own words: an escalated agent hears its
// own question back, which is what identifies the state it asked for.
func (e *Engine) standingBecause(project, agent, speech string) string {
	if speech == says.Escalated {
		if st, err := e.Store.For(project).GetState(agent); err == nil && st.Escalation != "" {
			return "you are ESCALATED on: " + st.Escalation + " — answer it, then `sindri resume`"
		}
	}
	return says.Refused(speech)
}

// Standing is where the machine has an agent and what that state offers, for the command surface.
// The registry asks rather than deriving: what an agent may type and where it stands are one
// question, and two answers to it is how a verb came to be gated by a copy of the rules.
//
// It reports where the agent WOULD land, not where it stands — a verb is typed before the agent's
// next pass, and offering it what a state it has already left allowed would be a moment out of date.
func (e *Engine) Standing(project, agent string) registry.Standing {
	s, err := e.flow.Would(project + "/" + agent)
	if err != nil {
		return registry.Standing{} // unplaceable: Known stays false and the closures decide alone
	}
	names := make([]string, 0, len(s.Verbs))
	for _, o := range s.Verbs {
		names = append(names, o.Verb.Name)
	}
	governs := make([]string, 0, len(verb.All))
	for _, v := range verb.All {
		governs = append(governs, v.Name)
	}
	return registry.Standing{State: s.Name, Verbs: names, Governs: governs,
		Because: e.standingBecause(project, agent, s.Says), Known: true}
}
