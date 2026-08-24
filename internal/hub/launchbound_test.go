package hub

import (
	"strings"
	"testing"
	"time"
)

// TestLaunchFailureLeavesAFreshLaunchAlone: podman may not have even created the container yet, so
// absence (or any other reading) this soon after the keystroke is not evidence of anything.
func TestLaunchFailureLeavesAFreshLaunchAlone(t *testing.T) {
	if got := launchFailure(0, false, false, false); got != "" {
		t.Errorf("launchFailure(0, ...) = %q, want no verdict yet", got)
	}
	if got := launchFailure(launchGrace-time.Second, true, true, false); got != "" {
		t.Errorf("launchFailure just under launchGrace = %q, want none", got)
	}
}

// TestLaunchFailureCatchesAnExitedContainer: a container that exists and has stopped is a failed
// start, not a slow one — the fastest and most definite of the three signals, so it fires regardless
// of how little time has passed (past the initial grace).
func TestLaunchFailureCatchesAnExitedContainer(t *testing.T) {
	got := launchFailure(launchGrace+time.Second, true, true, false)
	if got == "" {
		t.Fatal("an exited container during launch must be declared failed")
	}
	if !strings.Contains(got, "exited") {
		t.Errorf("reason = %q, want it to name the container exiting", got)
	}
}

// TestLaunchFailureNeedsAnObservationToCallItExited: the caller's inspect can time out, and the
// runtime too unwell to answer is the one that hangs launches in the first place. Silence must not
// be read as an exit — the time bounds are what catch that agent, a beat or two later.
func TestLaunchFailureNeedsAnObservationToCallItExited(t *testing.T) {
	if got := launchFailure(launchGrace+time.Second, true, false, false); got != "" {
		t.Errorf("unobserved container = %q, want no verdict — nothing was established", got)
	}
	if got := launchFailure(launchOverallBound+time.Second, true, false, false); got == "" {
		t.Error("the overall bound must still fire when the container was never observed")
	}
}

// TestLaunchFailureWaitsOutTheSessionBound: a container that is up and whose session has not
// answered YET is not a failure until launchSessionBound passes — the pod is there to inspect, and
// Launch's own wait (agent.launchReadyTimeout) covers the ordinary case well inside that bound.
func TestLaunchFailureWaitsOutTheSessionBound(t *testing.T) {
	if got := launchFailure(launchSessionBound-time.Second, true, false, false); got != "" {
		t.Errorf("under the session bound = %q, want no verdict yet", got)
	}
	got := launchFailure(launchSessionBound+time.Second, true, false, false)
	if got == "" {
		t.Fatal("a running container whose session never answered past the bound must fail")
	}
	if !strings.Contains(got, "session") {
		t.Errorf("reason = %q, want it to name the missing session", got)
	}
}

// TestLaunchFailureSessionUpNeverFiresOnTheSessionBound: once the session HAS answered, the
// session-specific bound must never fire on it — only the overall ceiling could still catch
// something else gone wrong, and nothing here has.
func TestLaunchFailureSessionUpNeverFiresOnTheSessionBound(t *testing.T) {
	got := launchFailure(launchSessionBound+time.Second, true, false, true)
	if got != "" {
		t.Errorf("session up = %q, want no verdict — the session answered", got)
	}
}

// TestLaunchFailureHasAnOverallCeiling: everything else — a wedged image build, a hung podman run,
// a container that never even gets created — is caught by the generous overall bound, the backstop
// for whatever the two sharper signals above did not name.
func TestLaunchFailureHasAnOverallCeiling(t *testing.T) {
	if got := launchFailure(launchOverallBound-time.Second, false, false, false); got != "" {
		t.Errorf("under the overall bound, container never even created = %q, want none yet", got)
	}
	got := launchFailure(launchOverallBound+time.Second, false, false, false)
	if got == "" {
		t.Fatal("a launch that outlived the overall bound must be declared failed")
	}
}
