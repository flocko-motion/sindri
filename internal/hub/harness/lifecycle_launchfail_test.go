package harness

import (
	"io"
	"strings"
	"testing"
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
	// The failure NAMES itself rather than falling silently to "down": a launch that would not start
	// is the verdict on itself, and the board must not read "launching" for one that has given up.
	l, f, _ := s.Intent("proj", "eitri")
	if l || !f {
		t.Errorf("intent = {launching:%v failed:%v}, want the failure standing and the launch over", l, f)
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
