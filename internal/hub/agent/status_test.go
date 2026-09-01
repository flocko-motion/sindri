package agent

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// intentFixture is one registered agent and the service holding its transient intent.
func intentFixture(t *testing.T) *Service {
	t.Helper()
	_, st := newService(t)
	s := New(st, clearTestDeps{}, nil)
	if err := st.For("proj").PutAgent(store.Agent{Name: "galar", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestIntentIsReportedAsFactsNotAWord: the harness hands over what was ASKED FOR, and what that
// means for the word an agent wears is folded by the orchestrator against everything else it knows
// (-> situation.Situation.Allowed). Three booleans, exactly one of which can be true.
func TestIntentIsReportedAsFactsNotAWord(t *testing.T) {
	s := intentFixture(t)
	if l, f, st := s.Intent("proj", "galar"); l || f || st {
		t.Errorf("Intent = (%v, %v, %v), want nothing asked for yet", l, f, st)
	}
	s.setLifecycle("proj", "galar", "launching")
	if l, f, st := s.Intent("proj", "galar"); !l || f || st {
		t.Errorf("Intent = (%v, %v, %v), want launching alone", l, f, st)
	}
	s.setLifecycle("proj", "galar", "stopping")
	if l, f, st := s.Intent("proj", "galar"); l || f || !st {
		t.Errorf("Intent = (%v, %v, %v), want stopping alone", l, f, st)
	}
	s.setLifecycle("proj", "galar", api.StatusLaunchFailed)
	if l, f, st := s.Intent("proj", "galar"); l || !f || st {
		t.Errorf("Intent = (%v, %v, %v), want the failed launch alone", l, f, st)
	}
}

// TestAnIntentSurvivesAnUnobservedAgent is the trap: up=false ALONE settles nothing, because an agent
// the watchdog has not reached yet supports no claim either way. Only a look that actually happened
// can retire a stop.
func TestAnIntentSurvivesAnUnobservedAgent(t *testing.T) {
	s := intentFixture(t)
	s.setLifecycle("proj", "galar", "stopping")

	s.SettleIntent("proj", "galar", false, false) // nothing has looked
	if _, _, stopping := s.Intent("proj", "galar"); !stopping {
		t.Error("an unobserved agent settled a stop it has no evidence for")
	}
	s.SettleIntent("proj", "galar", true, true) // looked, and it is still up
	if _, _, stopping := s.Intent("proj", "galar"); !stopping {
		t.Error("the pod is still up, so the stop has not been fulfilled")
	}
	s.SettleIntent("proj", "galar", false, true) // looked, and it is gone
	if _, _, stopping := s.Intent("proj", "galar"); stopping {
		t.Error("seen gone fulfils the stop, so the intent must be retired")
	}
}

// TestSeeingItUpSettlesALaunch is the other half, and the reason this is a WRITE rather than part of
// the fold: the word can be computed as often as anyone likes, but retiring an intent may happen once.
func TestSeeingItUpSettlesALaunch(t *testing.T) {
	s := intentFixture(t)
	s.setLifecycle("proj", "galar", "launching")

	s.SettleIntent("proj", "galar", false, true) // still not up: the request stands
	if launching, _, _ := s.Intent("proj", "galar"); !launching {
		t.Error("a launch that has not come up yet must keep its intent")
	}
	s.SettleIntent("proj", "galar", true, true)
	if launching, _, _ := s.Intent("proj", "galar"); launching {
		t.Error("the pod is up, so the launch is fulfilled and the intent is spent")
	}
}
