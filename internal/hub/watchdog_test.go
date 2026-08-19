package hub

import (
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/store"
)

// seen builds the pair one capture yields: what the pane said, and what it looked like. A distinct
// digest per call stands for a screen that changed since the last look.
func seen(runtime, digest string) agent.Observation {
	return agent.Observation{Runtime: runtime, Digest: digest}
}

// busy is seen with a tool call reported in flight — a shell whose output has not landed yet, so the
// digest holds still exactly as a stall would, and only this bit tells them apart.
func busy(runtime, digest string) agent.Observation {
	return agent.Observation{Runtime: runtime, Digest: digest, ToolRunning: true}
}

// TestOneLostProbeDoesNotFlipAnAgentDown is the bug the watchdog exists for. A probe that loses
// a race is not evidence: the readings before it said the agent was up, and one contended exec
// cannot outweigh them. Per-request probing could never do this — each request began with no
// history, so a single miss WAS the verdict, and a healthy agent flickered.
func TestOneLostProbeDoesNotFlipAnAgentDown(t *testing.T) {
	h := newHub(t)
	w := h.watch
	a := store.Agent{Project: "proj", Name: "galar"}

	w.record(a, true, 1, seen("working", "d1")) // a good reading first
	if l, _ := w.get("proj", "galar"); !l.up {
		t.Fatal("a successful probe must report up")
	}

	// Failures short of the threshold hold the previous state, dial-in count and runtime
	// included: a probe that could not read the agent must not blank what the last one did.
	for i := 1; i < downStrikes; i++ {
		w.record(a, false, 0, agent.Observation{})
		l, _ := w.get("proj", "galar")
		if !l.up {
			t.Errorf("strike %d of %d already reported down", i, downStrikes)
		}
		if l.clients != 1 || l.runtime != "working" {
			t.Errorf("strike %d blanked the last good detail: clients=%d runtime=%q", i, l.clients, l.runtime)
		}
	}

	// The threshold reached, it is no longer one bad reading but a pattern.
	w.record(a, false, 0, agent.Observation{})
	if l, _ := w.get("proj", "galar"); l.up {
		t.Errorf("%d consecutive failures should report down", downStrikes)
	}
}

// TestSuccessClearsStrikes: an agent that answers again starts from a clean slate, so an earlier
// rough patch cannot combine with a later one to declare a healthy agent down.
func TestSuccessClearsStrikes(t *testing.T) {
	h := newHub(t)
	w := h.watch
	a := store.Agent{Project: "proj", Name: "galar"}

	w.record(a, true, 0, seen("idle", "d1"))
	w.record(a, false, 0, agent.Observation{}) // one strike
	w.record(a, true, 2, seen("working", "d2"))
	if l, _ := w.get("proj", "galar"); l.strikes != 0 || l.clients != 2 || l.runtime != "working" {
		t.Errorf("a success must reset strikes and take the fresh reading, got %+v", l)
	}
	// From clean, it again takes the full threshold to go down.
	for i := 1; i < downStrikes; i++ {
		w.record(a, false, 0, agent.Observation{})
		if l, _ := w.get("proj", "galar"); !l.up {
			t.Errorf("strike %d after a success reported down too early", i)
		}
	}
}

// TestAbsentPodIsAStrikeLikeAnyOther: absence from a listing used to be conclusive, on the
// reasoning that a missing pod has nothing to be racing. It does: a listing describes the moment it
// was taken, so one that predates a launch reports the new pod absent and declared a running agent
// down from the weakest evidence available. A reading is a reading, whatever its source.
//
// The cost is deliberate — a genuinely stopped agent reads up for downStrikes sweeps rather than
// one. That is the same hysteresis a failed probe already gets, and it errs toward the state that
// self-corrects: a stale "up" is fixed by the next sweep, a false "down" is read by a human first.
func TestAbsentPodIsAStrikeLikeAnyOther(t *testing.T) {
	h := newHub(t)
	w := h.watch
	a := store.Agent{Project: "proj", Name: "galar"}

	w.record(a, true, 1, seen("working", "d1"))
	w.record(a, false, 0, agent.Observation{}) // absent from one listing — not yet a verdict
	l, _ := w.get("proj", "galar")
	if !l.up {
		t.Error("one listing that missed the pod must not declare the agent down")
	}
	if l.clients != 1 || l.runtime != "working" {
		t.Errorf("the last good detail should be held, got clients=%d runtime=%q", l.clients, l.runtime)
	}
	for i := 2; i <= downStrikes; i++ {
		w.record(a, false, 0, agent.Observation{})
	}
	if l, _ := w.get("proj", "galar"); l.up {
		t.Errorf("%d consecutive absences are a pattern and must report down", downStrikes)
	}
}

// TestUnobservedAgentIsNotReportedUp: an agent the watchdog has never seen has no observation,
// and the board must treat that as "not known to be up" rather than inventing liveness.
func TestUnobservedAgentIsNotReportedUp(t *testing.T) {
	h := newHub(t)
	if l, ok := h.watch.get("proj", "never-seen"); ok || l.up {
		t.Errorf("an unobserved agent must report no observation, got %+v ok=%v", l, ok)
	}
}

// TestStillnessDwellRunsFromTheLastChange: the dwell separates a pause from a stall, and it measures
// the SCREEN. A tool call leaves the pane untouched for as long as it runs, so "nothing changed
// since" is the fact to keep — and any change at all ends the spell, whatever the words say.
func TestStillnessDwellRunsFromTheLastChange(t *testing.T) {
	h := newHub(t)
	w := h.watch
	a := store.Agent{Project: "proj", Name: "dvalin"}

	w.record(a, true, 0, seen("working", "d1"))
	first, _ := w.get("proj", "dvalin")
	if first.stillSince.IsZero() {
		t.Fatal("the first reading has to start the clock — it is the last time the pane was known to change")
	}

	// The same screen again: the clock keeps running from when it stopped moving, not from this probe.
	w.record(a, true, 0, seen("working", "d1"))
	again, _ := w.get("proj", "dvalin")
	if !again.stillSince.Equal(first.stillSince) {
		t.Errorf("an unchanged pane must keep the dwell's start: %v then %v", first.stillSince, again.stillSince)
	}

	// It moved: the spell is over and a later stall is a new one, even though the word is unchanged.
	w.record(a, true, 0, seen("working", "d2"))
	moved, _ := w.get("proj", "dvalin")
	if !moved.stillSince.After(first.stillSince) {
		t.Error("a changed pane must restart the dwell")
	}
}

// TestAToolCallInFlightResetsTheDwellEvenWithNoOutputYet is the incident that started this: austri,
// running `make verify`, held one byte-identical pane for the whole gate and was nudged as stalled
// mid-command. A shell prints nothing until it exits, so the digest alone cannot tell that apart from
// a frozen turn — ToolRunning is the one thing that still says something is moving.
func TestAToolCallInFlightResetsTheDwellEvenWithNoOutputYet(t *testing.T) {
	h := newHub(t)
	w := h.watch
	a := store.Agent{Project: "proj", Name: "austri"}

	w.record(a, true, 0, seen("working", "d1"))
	first, _ := w.get("proj", "austri")

	// The gate is still running: same digest, same word, every beat — exactly what a stall looks
	// like, except the footer still shows a shell in flight.
	w.record(a, true, 0, busy("working", "d1"))
	stillRunning, _ := w.get("proj", "austri")
	if !stillRunning.stillSince.After(first.stillSince) {
		t.Error("a tool call reported in flight must reset the dwell, not let an unchanged pane read as stalled")
	}

	// It lands: the digest finally changes, output rendered, no shell left running.
	w.record(a, true, 0, seen("working", "d2"))
	landed, _ := w.get("proj", "austri")
	if !landed.stillSince.After(stillRunning.stillSince) {
		t.Error("the tool landing is itself a change and must move the dwell on again")
	}
}

// TestAToolCallPastTheCapStallsAgain is the other half of the same fix: the reset the marker earns
// is not a blank check. A shell that never returns is the freeze the dwell exists to catch, and
// resetting on the marker's mere presence for ever would hide exactly that. Past toolRunningCap the
// marker stops resetting the dwell on its own, and an unchanged pane reads as a stall again.
func TestAToolCallPastTheCapStallsAgain(t *testing.T) {
	h := newHub(t)
	w := h.watch
	a := store.Agent{Project: "proj", Name: "austri"}

	w.record(a, true, 0, seen("working", "d1"))
	key := agentKey{a.Project, a.Name}

	// Fake a marker that has already been in flight far longer than the cap allows — standing in
	// for the real wall-clock wait a wedged shell would otherwise take to reach here.
	w.mu.Lock()
	l := w.obs[key]
	l.toolSince = l.seen.Add(-(toolRunningCap + time.Minute))
	held := l.stillSince
	w.obs[key] = l
	w.mu.Unlock()

	// Same digest, marker still reported on, but past the cap: the reset must not fire.
	w.record(a, true, 0, busy("working", "d1"))
	if l, _ := w.get("proj", "austri"); !l.stillSince.Equal(held) {
		t.Error("past the cap, a marker that is merely still on must not keep resetting the dwell")
	}
}

// TestActivityDecidesWorkingWhenTheWordsDoNot: any screen the classifier does not recognise reads
// "idle", which made a busy agent with an unfamiliar pane look stopped. A pane that just changed is
// an agent doing something, whatever is written on it.
func TestActivityDecidesWorkingWhenTheWordsDoNot(t *testing.T) {
	h := newHub(t)
	w := h.watch
	a := store.Agent{Project: "proj", Name: "dvalin"}

	w.record(a, true, 0, seen("idle", "d1"))
	w.record(a, true, 0, seen("idle", "d2")) // same word, different screen
	if l, _ := w.get("proj", "dvalin"); l.runtime != "working" {
		t.Errorf("a changing pane is a working agent, got runtime %q", l.runtime)
	}
	// Standing still, it is idle again — and "blocked" is never overwritten, since a pane that
	// changes while asking a question is still asking it.
	w.record(a, true, 0, seen("idle", "d2"))
	if l, _ := w.get("proj", "dvalin"); l.runtime != "idle" {
		t.Errorf("an unchanged pane is idle, got %q", l.runtime)
	}
	w.record(a, true, 0, seen("blocked", "d3"))
	if l, _ := w.get("proj", "dvalin"); l.runtime != "blocked" {
		t.Errorf("activity must not overwrite a definite state, got %q", l.runtime)
	}
}

// TestALostProbeDoesNotRestartTheDwell: a failed probe holds the previous runtime, so it must hold
// the dwell too. Restarting the clock on every miss would hide a stall for another full dwell each
// time a probe lost a race — and the agent that stalls is often the one whose pod is contended.
func TestALostProbeDoesNotRestartTheDwell(t *testing.T) {
	h := newHub(t)
	w := h.watch
	a := store.Agent{Project: "proj", Name: "dvalin"}

	w.record(a, true, 0, seen("idle", "d1"))
	started, _ := w.get("proj", "dvalin")

	w.record(a, false, 0, agent.Observation{}) // one lost probe, short of downStrikes
	held, _ := w.get("proj", "dvalin")
	if held.runtime != "idle" {
		t.Fatalf("a lost probe should hold the last runtime, got %q", held.runtime)
	}
	// A capture that failed saw no screen, so it is not evidence the screen changed either.
	if !held.stillSince.Equal(started.stillSince) {
		t.Errorf("the dwell restarted on a lost probe: %v then %v", started.stillSince, held.stillSince)
	}
}

// TestTheBoardCarriesTheLastMemoryReading: the front-ends must never measure the host themselves,
// so the figure has to reach them on the board — and it comes from the watchdog's last reading,
// like liveness, rather than from a process spawn on every board read.
func TestTheBoardCarriesTheLastMemoryReading(t *testing.T) {
	const gib = int64(1) << 30
	h := newHub(t)

	// Nothing sampled yet: unknown, which renders as nothing rather than as a full machine.
	board, err := h.State("")
	if err != nil {
		t.Fatal(err)
	}
	if board.Memory.Known() {
		t.Errorf("an unsampled hub reported a figure: %+v", board.Memory)
	}

	h.watch.mu.Lock()
	h.watch.capacity = container.Capacity{UsedBytes: 6 * gib, TotalBytes: 16 * gib, Basis: container.BasisInUse}
	h.watch.mu.Unlock()

	if board, err = h.State(""); err != nil {
		t.Fatal(err)
	}
	if board.Memory.FreeBytes() != 10*gib {
		t.Errorf("board free = %d bytes, want the 10 GiB the reading leaves", board.Memory.FreeBytes())
	}
}
