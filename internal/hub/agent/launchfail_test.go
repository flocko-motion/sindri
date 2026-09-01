package agent

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/container"
)

// TestLaunchRecordsTheRequestEvenWhenThePreflightFails is the ordering fix sd-89de80 asked for:
// the "launch: requested" log entry (and the intent behind it) must land before container.Check
// runs, not after — otherwise a slow preflight (on macOS, a stopped podman VM starting) leaves the
// board reading "down" for having asked nothing yet, when a launch is already under way.
func TestLaunchRecordsTheRequestEvenWhenThePreflightFails(t *testing.T) {
	s, _ := tellFixture(t, "eitri", idlePane) // fakeRuntime.Check always fails
	err := s.Launch(t.Context(), "proj", "eitri", false, false, 0, 0, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "nothing to launch into") {
		t.Fatalf("Launch = %v, want it to fail at the fake preflight", err)
	}
	evs, everr := s.store.For("proj").Events("eitri", 0)
	if everr != nil {
		t.Fatalf("events: %v", everr)
	}
	found := false
	for _, e := range evs {
		if e.Type == "launch" && e.Payload == "requested" {
			found = true
		}
	}
	if !found {
		t.Errorf("no 'launch: requested' entry — the request was not recorded ahead of the failing preflight, events=%v", evs)
	}
	// The failure still clears the intent, same as before the reordering: nothing here should
	// leave the board stuck reading "launching" for a launch that has already given up.
	if l, f, _ := s.Intent("proj", "eitri"); l || f {
		t.Error("a failed preflight must leave no intent standing, or the board reads launching for ever")
	}
}

// TestAFailedLaunchSaysSoInTheLog: three of eitri's launches left "requested" as the last word in
// its log and nothing after, which reads exactly like a launch still in flight — the error went to
// the caller alone, and the log is where a failure days old is reconstructed.
func TestAFailedLaunchSaysSoInTheLog(t *testing.T) {
	s, _ := tellFixture(t, "eitri", idlePane) // fakeRuntime.Check always fails
	if err := s.Launch(t.Context(), "proj", "eitri", false, false, 0, 0, io.Discard); err == nil {
		t.Fatal("Launch succeeded against the failing fake preflight")
	}
	evs, everr := s.store.For("proj").Events("eitri", 0)
	if everr != nil {
		t.Fatalf("events: %v", everr)
	}
	for _, e := range evs {
		if e.Type == "launch" && strings.HasPrefix(e.Payload, "failed:") {
			if !strings.Contains(e.Payload, "nothing to launch into") {
				t.Errorf("the failure entry %q does not carry the reason", e.Payload)
			}
			return
		}
	}
	t.Errorf("no 'launch: failed' entry — a launch that gave up is indistinguishable from one still running, events=%v", evs)
}

// TestLaunchIntentTracksOnlyLaunching: the watchdog's bound applies to a launch in flight, and
// nothing else the same map holds — a stop intent, or none at all, must not be mistaken for one.
func TestLaunchIntentTracksOnlyLaunching(t *testing.T) {
	s, _ := newService(t)
	if _, ok := s.LaunchIntent("proj", "eitri"); ok {
		t.Error("no intent set at all should not read as launching")
	}
	s.setLifecycle("proj", "eitri", "launching")
	since, ok := s.LaunchIntent("proj", "eitri")
	if !ok || since.IsZero() {
		t.Errorf("LaunchIntent = (%v, %v), want a real timestamp and ok", since, ok)
	}
	s.setLifecycle("proj", "eitri", "stopping")
	if _, ok := s.LaunchIntent("proj", "eitri"); ok {
		t.Error("a stop intent must not read as a launch in flight")
	}
}

// TestFailLaunchMarksLogsAndReleases covers sd-c6c4aa's "on failure" list: a distinct status (not
// a silent fall to "down"), the reason logged beside the request it answers, and the container —
// if podman still has one — removed so its memory reservation is released.
func TestFailLaunchMarksLogsAndReleases(t *testing.T) {
	s, f := tellFixture(t, "eitri", idlePane)
	s.setLifecycle("proj", "eitri", "launching")

	s.FailLaunch(context.Background(), "proj", "eitri", "the container exited during launch")

	if _, failed, _ := s.Intent("proj", "eitri"); !failed {
		t.Error("FailLaunch must leave the failed-launch intent standing, for the board to fold")
	}
	evs, _ := s.store.For("proj").Events("eitri", 0)
	found := false
	for _, e := range evs {
		if e.Type == "launch" && strings.Contains(e.Payload, "failed: the container exited during launch") {
			found = true
		}
	}
	if !found {
		t.Errorf("no logged reason for the failure, events=%v", evs)
	}
	if len(f.removed) != 1 || f.removed[0] != "pod-eitri" {
		t.Errorf("removed = %v, want the container released exactly once", f.removed)
	}

	// A second call must not pile on: the intent is no longer "launching", so nothing here is
	// this call's to fail a second time.
	s.FailLaunch(context.Background(), "proj", "eitri", "a different reason")
	if len(f.removed) != 1 {
		t.Errorf("removed = %v, want no second removal once already failed", f.removed)
	}
}

// TestFailLaunchIsANoOpOnceTheLaunchHasMovedOn: a launch that already came up (or was never in
// flight) must not be retroactively marked failed by a stray, late call.
func TestFailLaunchIsANoOpOnceTheLaunchHasMovedOn(t *testing.T) {
	s, f := tellFixture(t, "eitri", idlePane)
	// No "launching" intent at all — e.g. it already resolved by the time the watchdog got here.
	s.FailLaunch(context.Background(), "proj", "eitri", "too late")
	if _, failed, _ := s.Intent("proj", "eitri"); failed {
		t.Error("FailLaunch marked an agent that was never recorded as launching")
	}
	if len(f.removed) != 0 {
		t.Errorf("removed = %v, want nothing torn down for a launch that was not in flight", f.removed)
	}
}

// TestTheSweepsVerdictOutlivesTheLaunchCall: the two mechanisms overlap in time — Launch waits
// launchReadyTimeout for a session the sweep has already given up on, and then returns an error of
// its own. Its cleanup may retract only the intent it set, or the reason the sweep found is
// replaced by "down", which says nobody asked.
func TestTheSweepsVerdictOutlivesTheLaunchCall(t *testing.T) {
	s, _ := tellFixture(t, "eitri", idlePane)
	s.setLifecycle("proj", "eitri", "launching")
	s.FailLaunch(context.Background(), "proj", "eitri", "the container exited during launch")

	s.clearLaunching("proj", "eitri")

	if _, failed, _ := s.Intent("proj", "eitri"); !failed {
		t.Error("the sweep's verdict must outlive the launch call that raced it")
	}
	// A launch still in flight is the case the cleanup IS for: that one it must retract.
	s.setLifecycle("proj", "galar", "launching")
	s.clearLaunching("proj", "galar")
	if l, f, _ := s.Intent("proj", "galar"); l || f {
		t.Error("clearLaunching must retract the intent the call itself set")
	}
}

// wedgedRuntime is a backend whose teardown only ever reports its deadline — the runtime that hung
// the launch being asked to undo it.
type wedgedRuntime struct{ *fakeRuntime }

func (wedgedRuntime) RmContext(ctx context.Context, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

// TestTheVerdictIsWrittenBeforeTheRuntimeIsAskedAnything: the status and the reason are the point of
// this state, and the runtime being asked to release the container is the one that just failed to
// start it. So they are recorded first, and a removal that never answers costs the user nothing but
// a line saying the container is still there.
func TestTheVerdictIsWrittenBeforeTheRuntimeIsAskedAnything(t *testing.T) {
	s, f := tellFixture(t, "eitri", idlePane)
	container.Use(wedgedRuntime{f})
	s.setLifecycle("proj", "eitri", "launching")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	s.FailLaunch(ctx, "proj", "eitri", "the container exited during launch")

	if _, failed, _ := s.Intent("proj", "eitri"); !failed {
		t.Error("the verdict is written before the runtime is asked anything, so it must stand here")
	}
	evs, _ := s.store.For("proj").Events("eitri", 0)
	var reason, unreleased bool
	for _, e := range evs {
		if strings.Contains(e.Payload, "failed: the container exited during launch") {
			reason = true
		}
		if strings.Contains(e.Payload, "container not released") {
			unreleased = true
		}
	}
	if !reason || !unreleased {
		t.Errorf("events=%v, want both the reason and the abandoned removal on the record", evs)
	}
}
