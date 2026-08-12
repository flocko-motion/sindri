package cli

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestCoauthorTreatsUnobservedAsNeedingLaunch is the flow that actually broke. ensureCoauthor
// creates the agent and returns at once, and the watchdog only looks every couple of seconds — so
// "not observed yet" is the ORDINARY state of a coauthor one line after creating it. Reading that
// as running skipped the launch and handed the terminal to a pod that was never started.
//
// This pins the two decisions the flow makes, through the real statusOf; driving
// ensureCoauthorAlive itself would need a live hub.
func TestCoauthorTreatsUnobservedAsNeedingLaunch(t *testing.T) {
	st := api.BoardState{Agents: []api.AgentView{
		{Project: "proj", Name: "co", Status: api.StatusUnknown},
	}}
	if !api.AgentNeedsLaunch(statusOf(st, "proj", "co")) {
		t.Error("a just-created coauthor must still be launched — this is the skipped-launch bug")
	}
	// And the wait loop must keep waiting rather than declaring the session live.
	if !api.AgentNotUp(statusOf(st, "proj", "co")) {
		t.Error("the readiness loop must not accept an unobserved agent as ready")
	}
}

// TestCoauthorDoesNotRelaunchARunningOne guards the other side: a coauthor already collaborating is
// reattached, not launched again, which is why the launch test is the narrower predicate.
func TestCoauthorDoesNotRelaunchARunningOne(t *testing.T) {
	st := api.BoardState{Agents: []api.AgentView{
		{Project: "proj", Name: "co", Status: "collab"},
	}}
	if api.AgentNeedsLaunch(statusOf(st, "proj", "co")) {
		t.Error("a running coauthor must not be relaunched")
	}
	if api.AgentNotUp(statusOf(st, "proj", "co")) {
		t.Error("a running coauthor is ready; the wait loop should return")
	}
}

// TestCoauthorWaitsOutALaunchInFlight: launching is not a reason to launch again, but it is a
// reason to keep waiting — the two predicates differ exactly here.
func TestCoauthorWaitsOutALaunchInFlight(t *testing.T) {
	st := api.BoardState{Agents: []api.AgentView{
		{Project: "proj", Name: "co", Status: "launching"},
	}}
	if api.AgentNeedsLaunch(statusOf(st, "proj", "co")) {
		t.Error("a launch already in flight must not be started a second time")
	}
	if !api.AgentNotUp(statusOf(st, "proj", "co")) {
		t.Error("a launch in flight is not ready yet")
	}
}
