package situation

import (
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/observe"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestStalledOnlyCountsHeldWork is the whole rule. The phases that exist to wait must never read as
// stalled, or the nudge becomes noise on exactly the agents behaving correctly — and the evidence is
// the screen standing still, not the word printed on it.
func TestStalledOnlyCountsHeldWork(t *testing.T) {
	past, under := StallDwell+time.Minute, StallDwell-time.Minute
	for _, c := range []struct {
		what                      string
		phase, container, runtime string
		waitingOnRun              bool
		stillFor                  time.Duration
		want                      bool
	}{
		{"holding work and gone quiet", "working", "", "idle", false, past, true},
		{"quiet, but not for long enough", "working", "", "idle", false, under, false},
		// The case the old rule could not see: a turn that wedged leaves "esc to interrupt" on screen
		// forever, so the classifier says "working" while not one byte changes for minutes.
		{"still saying 'working', with a frozen screen", "working", "working", "working", false, past, true},
		{"asking for input — that is 'blocked', already visible", "working", "", "blocked", false, past, false},
		{"signed out: motionless because it cannot act, and no prod reaches it", "working", "", "signed-out", false, past, false},
		{"waiting for a verdict on a submitted PR", "submitted", "", "idle", false, past, false},
		{"waiting for a verdict on a feature's PR", "submitted", "td-EPIC", "idle", false, past, false},
		// A finished feature is the worker's to submit, so parking on one is a stall. It was excluded
		// while only a human could open the milestone PR, and that wait no longer exists.
		{"a feature whose subtasks are all checkpointed", "idle", "td-EPIC", "idle", false, past, true},
		{"mid-feature, on a subtask, gone quiet", "working", "td-EPIC", "idle", false, past, true},
		{"between assignments, holding nothing", "idle", "", "idle", false, past, false},
		// ori's case: assignReview writes "reviewing" and nothing else, so neither of the old disjuncts
		// could ever hold and a reviewer that stopped reading was invisible to every sweep.
		{"a reviewer holding a PR, gone quiet", "reviewing", "", "idle", false, past, true},
		{"a reviewer quiet, but not for long enough", "reviewing", "", "idle", false, under, false},
		{"a reviewer asking the user something", "reviewing", "", "blocked", false, past, false},
		// An unreadable pane is not a reading, so the words are empty — but the dwell it carries is
		// still time in which nothing was seen to change, and holding work through that is a stall.
		{"unreadable pane, work held, nothing seen to move", "working", "", "", false, past, true},
		// The queue is the hub's, not the agent's: a worker parked on its own gate run, or a reviewer
		// on a lint-pr it asked for, is correctly motionless — the nudge would tell it to carry on
		// with the very thing the hub itself is holding.
		{"a worker's own gate run is queued or running", "working", "", "idle", true, past, false},
		{"a reviewer waiting on a lint-pr run it asked for", "reviewing", "", "idle", true, past, false},
	} {
		s := Situation{
			Phase: c.phase, Container: c.container, WaitingOnRun: c.waitingOnRun,
			Observation: observe.Observation{State: observe.ParseState(c.runtime)}, StillFor: c.stillFor,
		}
		if got := s.Allowed().Stalled; got != c.want {
			t.Errorf("%s: Stalled(%q, %q, %q, %v, %v) = %v, want %v",
				c.what, c.phase, c.container, c.runtime, c.waitingOnRun, c.stillFor, got, c.want)
		}
	}
}

// TestACutOffTurnIsRetriedInAnyPhase is gloin's case: the API stalled its response mid-stream, the
// pane kept its "esc to interrupt" footer and kept redrawing, and every other signal read a live
// turn. Nothing resumes on its own, so this is not a judgement about idleness — it counts wherever
// the agent is, including waiting on a verdict it could not act on anyway.
func TestACutOffTurnIsRetriedInAnyPhase(t *testing.T) {
	cutOff := func(phase string, still time.Duration) bool {
		return Situation{Phase: phase, Observation: observe.Observation{State: observe.TurnCutOff}, StillFor: still}.Allowed().Stalled
	}
	for _, phase := range []string{"working", "submitted", "idle", "resolving"} {
		if !cutOff(phase, RetryDwell+time.Second) {
			t.Errorf("phase %q: a cut-off turn must be retried", phase)
		}
		// Not instantly, though: a retry already in flight gets to finish first.
		if cutOff(phase, RetryDwell-time.Second) {
			t.Errorf("phase %q: retried before the dwell elapsed", phase)
		}
	}
	// And it is quicker than a stall, which needs evidence rather than a stated fact.
	if RetryDwell >= StallDwell {
		t.Errorf("RetryDwell %v should be shorter than StallDwell %v", RetryDwell, StallDwell)
	}
}

// TestOneRuleOneAnswer is the point of the whole surface: the question "may this agent be handed
// work" reaches every caller through one derivation, so a retired agent is refused by the nudge
// sweeps exactly as it is by the assignment path. Two of them disagreed, and a retired agent was
// pushed "ready for you" every thirty seconds (-> sd-daf9c7).
func TestOneRuleOneAnswer(t *testing.T) {
	retired := Situation{Name: "dvalin", Role: "worker", Retired: true, Phase: "idle"}
	allowed := retired.Allowed()
	if allowed.Wake == "" {
		t.Fatal("a retired agent must not be woken toward work it will be refused")
	}
	if allowed.Assign != allowed.Wake || allowed.Nudge != allowed.Wake {
		t.Errorf("one rule, three answers: assign=%q nudge=%q wake=%q", allowed.Assign, allowed.Nudge, allowed.Wake)
	}
	if !strings.Contains(allowed.Nudge, "retire dvalin --back") {
		t.Errorf("the refusal %q must name the way out of it", allowed.Nudge)
	}
}

// TestARefusalNamesWhatIsHeld: a reason is what the agent is told, so it has to identify the work
// rather than state that some exists.
func TestARefusalNamesWhatIsHeld(t *testing.T) {
	held := Situation{Role: "worker", Phase: "idle", AwaitingPR: "pr-sd-9", AwaitingTask: "sd-9"}
	if why := held.Allowed().Assign; !strings.Contains(why, "pr-sd-9") || !strings.Contains(why, "sd-9") {
		t.Errorf("Assign = %q, want it to name both the task and the PR still to land", why)
	}
	if held.AtLeafBoundary() {
		t.Error("an unlanded PR is work in flight — resetting the session there loses what it waits for")
	}
}

// TestAHeldFeatureIsABoundaryButNotIdle is the distinction between the two questions: a reset
// between subtasks is safe, while reclaiming the pod under a held feature is not.
func TestAHeldFeatureIsABoundaryButNotIdle(t *testing.T) {
	between := Situation{Role: "worker", Container: "td-EPIC", Phase: "idle"}
	if !between.AtLeafBoundary() {
		t.Error("between subtasks IS a leaf boundary — the clear fires exactly there")
	}
	if between.HoldsNothing() {
		t.Error("a held feature is work: reclaiming its pod would throw the branch's session away")
	}
}

// TestACoauthorIsNeverReclaimed: its session is the user's own seat, whatever it holds.
func TestACoauthorIsNeverReclaimed(t *testing.T) {
	if (Situation{Role: "coauthor"}).HoldsNothing() {
		t.Error("a coauthor's pod is the user's seat, not an idle one to take back")
	}
}

// TestNeedsUserFollowsTheBoardsOwnWord: the marker and the status cannot disagree, because both are
// folded here from the same evidence rather than derived twice. Each case states the EVIDENCE and
// asserts the word it produces, so a change to the fold is caught by the same test as the marker.
func TestNeedsUserFollowsTheBoardsOwnWord(t *testing.T) {
	seen := time.Now()
	for _, c := range []struct {
		what string
		obs  observe.Observation
		want string
	}{
		{"a question at its prompt", observe.Observation{TakenAt: seen, Up: true, State: observe.AwaitingHuman}, api.StatusBlocked},
		{"a /login banner", observe.Observation{TakenAt: seen, Up: true, State: observe.SignedOut}, api.StatusSignedOut},
		{"a launch that never came up", observe.Observation{TakenAt: seen, LaunchFailed: true}, api.StatusLaunchFailed},
	} {
		got := (Situation{Observation: c.obs}).Allowed()
		if got.Status != c.want {
			t.Errorf("%s: status = %q, want %q", c.what, got.Status, c.want)
		}
		if !got.NeedsUser {
			t.Errorf("%s: %q resolves only if a human acts", c.what, got.Status)
		}
	}
	// Idle is nobody's problem — the next ask reassigns it.
	idle := (Situation{Observation: observe.Observation{TakenAt: seen, Up: true, State: observe.AtPrompt}}).Allowed()
	if idle.Status != "idle" || idle.NeedsUser {
		t.Errorf("an agent at an empty prompt reads %q, needsUser=%v — want idle and nobody's problem",
			idle.Status, idle.NeedsUser)
	}
}

// TestTheWordFollowsTheIntentUntilRealityCatchesUp: a launch asked for reads "launching" until the
// pod is seen, and nothing observed at all is "unknown" rather than "down" — a claim no evidence
// supports is the error this fold exists to avoid.
func TestTheWordFollowsTheIntentUntilRealityCatchesUp(t *testing.T) {
	seen := time.Now()
	for _, c := range []struct {
		what string
		s    Situation
		want string
	}{
		{"asked to start, not up yet", Situation{Observation: observe.Observation{TakenAt: seen, Launching: true}}, "launching"},
		{"asked to stop, still up", Situation{Observation: observe.Observation{TakenAt: seen, Up: true, Stopping: true}}, "stopping"},
		{"asked to stop, now gone", Situation{Observation: observe.Observation{TakenAt: seen, Stopping: true}}, "down"},
		{"nothing has looked yet", Situation{}, "unknown"},
		{"torn down on purpose", Situation{Stopped: true, Observation: observe.Observation{TakenAt: seen}}, "stopped"},
		{"up, holding a task", Situation{Phase: "working", Observation: observe.Observation{TakenAt: seen, Up: true}}, "working"},
	} {
		if got := c.s.Allowed().Status; got != c.want {
			t.Errorf("%s: status = %q, want %q", c.what, got, c.want)
		}
	}
}

// TestParkedByTheHubReadsTheGateOnce: a feature worker waiting on a verdict one level down is parked
// deliberately, and the gated set comes off the pool the situation already carries.
func TestParkedByTheHubReadsTheGateOnce(t *testing.T) {
	pool := Pool{All: []store.Task{
		{ID: "td-EPIC", Status: "open"},
		{ID: "td-1", ParentID: "td-EPIC", Status: "open", Approval: "pending"},
	}}
	parked := Situation{Role: "worker", Container: "td-EPIC", Phase: "idle", Pool: pool}
	if !parked.ParkedByTheHub() {
		t.Error("its next subtask waits on the user; prodding it complains about the hub's own doing")
	}
	free := parked
	free.Pool = Pool{All: []store.Task{{ID: "td-EPIC", Status: "open"}}}
	if free.ParkedByTheHub() {
		t.Error("nothing gated under it, so it is idle on its own account")
	}
}
