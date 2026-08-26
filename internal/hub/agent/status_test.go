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

// TestPeekStatusDoesNotRetireASettledIntent: PeekStatus exists for an observer that must not decide
// when a launch/stop intent retires (-> statuswatch.go) — the word must match AgentStatus's, but the
// intent behind it must survive, however many times PeekStatus is called.
func TestPeekStatusDoesNotRetireASettledIntent(t *testing.T) {
	s := &Service{lifecycle: map[lcKey]lifecycleIntent{{"proj", "galar"}: {state: "stopping"}}}
	// Observed and down: the stop intent is now fulfilled, which AgentStatus would retire.
	if got := s.PeekStatus("proj", "galar", false, true, "", false); got != "down" {
		t.Errorf("peek reads %q, want down", got)
	}
	if _, still := s.lifecycle[lcKey{"proj", "galar"}]; !still {
		t.Fatal("PeekStatus must not retire a settled intent")
	}
	// Calling it again must read exactly the same word — nothing about the intent moved.
	if got := s.PeekStatus("proj", "galar", false, true, "", false); got != "down" {
		t.Errorf("a repeat peek reads %q, want down", got)
	}
	if _, still := s.lifecycle[lcKey{"proj", "galar"}]; !still {
		t.Fatal("a repeat PeekStatus must still not retire the intent")
	}
	// AgentStatus itself is still the one that retires it, once actually called.
	if got := s.AgentStatus("proj", "galar", false, true, "", false); got != "down" {
		t.Errorf("AgentStatus reads %q, want down", got)
	}
	if _, still := s.lifecycle[lcKey{"proj", "galar"}]; still {
		t.Error("AgentStatus should retire the intent PeekStatus left alone")
	}
}

// TestAgentStatusAndPeekStatusAgree: the two must answer identically for every case the switch in
// foldStatus branches on — they are the same fold, one of them just skips the write.
func TestAgentStatusAndPeekStatusAgree(t *testing.T) {
	cases := []struct {
		name                       string
		intent                     string
		running, observed, stopped bool
		phase                      string
	}{
		{"stopping, still up", "stopping", true, true, false, ""},
		{"stopping, observed down", "stopping", false, true, false, ""},
		{"stopping, unobserved", "stopping", false, false, false, ""},
		{"running with phase", "", true, true, false, "working"},
		{"running no phase", "", true, true, false, ""},
		{"launching", "launching", false, true, false, ""},
		{"launch failed", api.StatusLaunchFailed, false, true, false, ""},
		{"unobserved", "", false, false, false, ""},
		{"stopped", "", false, true, true, ""},
		{"down", "", false, true, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lc := map[lcKey]lifecycleIntent{}
			if c.intent != "" {
				lc[lcKey{"proj", "galar"}] = lifecycleIntent{state: c.intent}
			}
			peek := &Service{lifecycle: lc}
			gotPeek := peek.PeekStatus("proj", "galar", c.running, c.observed, c.phase, c.stopped)

			lc2 := map[lcKey]lifecycleIntent{}
			if c.intent != "" {
				lc2[lcKey{"proj", "galar"}] = lifecycleIntent{state: c.intent}
			}
			real := &Service{lifecycle: lc2}
			gotReal := real.AgentStatus("proj", "galar", c.running, c.observed, c.phase, c.stopped)

			if gotPeek != gotReal {
				t.Errorf("PeekStatus=%q AgentStatus=%q, want equal", gotPeek, gotReal)
			}
		})
	}
}
