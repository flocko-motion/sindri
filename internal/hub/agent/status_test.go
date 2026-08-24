package agent

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestUnobservedIsNotDown: "down" is a claim, and an agent the watchdog has not reached yet
// supports no claim. The board built `running` as a []bool, so an agent registered since the last
// sweep arrived here as running=false — a zero value reading as a negative observation, which is
// the same error as trusting a listing taken before the pod existed.
func TestUnobservedIsNotDown(t *testing.T) {
	s := &Service{lifecycle: map[lcKey]lifecycleIntent{}}
	if got := s.AgentStatus("proj", "galar", false, false, "", false); got != "unknown" {
		t.Errorf("an agent nothing has looked at reads %q, want unknown", got)
	}
	// Observed and not running IS a claim, and still reads down.
	if got := s.AgentStatus("proj", "galar", false, true, "", false); got != "down" {
		t.Errorf("an observed, absent agent reads %q, want down", got)
	}
}

// TestStoppedReadsDistinctFromDown: a human's StopAgent is a deliberate, resumable act, not the
// same claim "down" makes — a status shared with a crash would read a chosen state as a fault.
func TestStoppedReadsDistinctFromDown(t *testing.T) {
	s := &Service{lifecycle: map[lcKey]lifecycleIntent{}}
	if got := s.AgentStatus("proj", "galar", false, true, "", true); got != "stopped" {
		t.Errorf("a deliberately stopped agent reads %q, want stopped", got)
	}
	// Unobserved still outranks the durable flag: nothing has looked yet, so nothing confirms it.
	if got := s.AgentStatus("proj", "galar", false, false, "", true); got != "unknown" {
		t.Errorf("an unobserved agent reads %q, want unknown even when stopped is set", got)
	}
}

// TestIntentSurvivesAnUnobservedAgent: the launch and stop intents are cleared by reality catching
// up with them. Nothing has caught up when nothing has looked, so an unobserved reading must not
// retire an intent — that would drop the one word explaining what the user just asked for.
func TestIntentSurvivesAnUnobservedAgent(t *testing.T) {
	s := &Service{lifecycle: map[lcKey]lifecycleIntent{{"proj", "galar"}: {state: "stopping"}}}
	if got := s.AgentStatus("proj", "galar", false, false, "", false); got != "stopping" {
		t.Errorf("an unobserved agent under a stop intent reads %q, want stopping", got)
	}
	if s.lifecycle[lcKey{"proj", "galar"}].state != "stopping" {
		t.Error("the stop intent was retired on an observation that never happened")
	}
	// Once something has actually looked and found it gone, the intent is fulfilled.
	if got := s.AgentStatus("proj", "galar", false, true, "", false); got != "down" {
		t.Errorf("an observed stop reads %q, want down", got)
	}
	if _, still := s.lifecycle[lcKey{"proj", "galar"}]; still {
		t.Error("a fulfilled stop intent should be cleared")
	}
}

// TestLaunchingOutranksUnknown: a launch in flight already explains why the pod is not up, and it
// is the more useful of the two words — the user asked for it and is waiting on it.
func TestLaunchingOutranksUnknown(t *testing.T) {
	s := &Service{lifecycle: map[lcKey]lifecycleIntent{{"proj", "galar"}: {state: "launching"}}}
	if got := s.AgentStatus("proj", "galar", false, false, "", false); got != "launching" {
		t.Errorf("a launch in flight reads %q, want launching", got)
	}
}

// TestLaunchFailedOutranksUnknown: a failed launch is a claim like "launching" is, not a gap in
// observation — it must read as such regardless of whether anything has looked since.
func TestLaunchFailedOutranksUnknown(t *testing.T) {
	s := &Service{lifecycle: map[lcKey]lifecycleIntent{{"proj", "galar"}: {state: api.StatusLaunchFailed}}}
	if got := s.AgentStatus("proj", "galar", false, false, "", false); got != api.StatusLaunchFailed {
		t.Errorf("a failed launch reads %q, want %q", got, api.StatusLaunchFailed)
	}
	// Recovers like any other intent does: if the pod turns out to be running after all, that
	// wins outright and the stale failure verdict is cleared with it.
	if got := s.AgentStatus("proj", "galar", true, true, "", false); got != "idle" {
		t.Errorf("a running agent still marked failed reads %q, want idle", got)
	}
}

// TestRunningStillReportsThePhase guards the ordinary path: once observed running, the agent's own
// phase is the answer, and the observed flag changes nothing about it.
func TestRunningStillReportsThePhase(t *testing.T) {
	s := &Service{lifecycle: map[lcKey]lifecycleIntent{}}
	if got := s.AgentStatus("proj", "galar", true, true, "working", false); got != "working" {
		t.Errorf("a running agent reads %q, want its phase", got)
	}
	if got := s.AgentStatus("proj", "galar", true, true, "", false); got != "idle" {
		t.Errorf("a running agent with no phase reads %q, want idle", got)
	}
}
