// package: hub/flow/agent / pod
// type:    logic (a human's request for an agent's pod)
// job:     record that a human asked for a pod to come up or go away, and announce it. Neither
// starts nor stops anything: the machine's launching and stopping states do that, and this is the
// fact their conditions read.
// limits:  the record and the announcement. What a pod IS belongs to the harness, and when to act
// on the record belongs to the role's own map.
package agent

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/hub/flow/topic"
)

// AskStart records a human's request for this agent's pod. A REQUEST rather than a flag: the agent
// may already be up, so "start it" is an instruction and no flag on the row would tell it apart from
// the state the agent is in. The machine takes the request back when it carries it out.
func (a *Act) AskStart(project, name string) error { return a.askPod(project, name, true) }

// AskStop is the same for taking a pod back, whatever the agent is holding — a human's call, where
// the idle reclaim is the fleet's.
func (a *Act) AskStop(project, name string) error { return a.askPod(project, name, false) }

// askPod writes one of the two requests and announces it. It performs nothing: the machine's own
// launching and stopping states do that, and a request left standing is read again on the agent's
// next beat — which is what makes "asked for, and not yet done" survive a hub that died between.
func (a *Act) askPod(project, name string, start bool) error {
	ps := a.Store.For(project)
	ag, ok, err := ps.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	ag.StartAsked, ag.StopAsked = start, !start
	if err := ps.PutAgent(ag); err != nil {
		return err
	}
	what := "stop"
	if start {
		what = "start"
	}
	_ = ps.Log(name, what+"-asked", "a human asked for this pod")
	a.Deps.Notify()
	a.Flow.Wake(project, name, topic.AgentParked)
	return nil
}

// AnswerPodRequest takes back both requests, for whoever carried one out. Written here rather than
// beside each caller for the same reason the arming's disarm is: a request left standing brings the
// agent back into the state that answers it on every beat, for ever.
func (a *Act) AnswerPodRequest(project, name string) {
	ps := a.Store.For(project)
	ag, ok, err := ps.GetAgent(name)
	if err != nil || !ok || (!ag.StartAsked && !ag.StopAsked) {
		return
	}
	ag.StartAsked, ag.StopAsked = false, false
	if perr := ps.PutAgent(ag); perr != nil {
		fmt.Fprintf(os.Stderr, "hub: answering %s's pod request: %v\n", name, perr)
	}
}

// Settle runs the agent's own machine now, for a caller holding the line: a human who typed `stop`
// is answered after the pod has gone rather than before.
func (a *Act) Settle(project, name string) { a.Flow.Look(project, name) }
