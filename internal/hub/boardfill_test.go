package hub

import (
	"testing"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/store"
)

// stillWatchdog stops the sweeping observer and leaves a still one behind, so what a test records is
// what the board reads rather than a sweep that landed halfway through it.
func stillWatchdog(t *testing.T, h *Hub) *watchdog {
	t.Helper()
	h.watch.close()
	w := &watchdog{h: h, obs: map[agentKey]liveness{}, repos: map[string]repoSample{},
		stop: make(chan struct{}), done: make(chan struct{})}
	close(w.done) // nothing is running, so Hub.Close's close() returns instead of waiting for a sweep
	h.watch = w
	return w
}

// observedAgent registers one worker and hands back the row the observer records against.
func observedAgent(t *testing.T, h *Hub, name, model string) store.Agent {
	t.Helper()
	if err := h.store.For(testProject).PutAgent(store.Agent{Name: name, Role: "worker", Model: model}); err != nil {
		t.Fatal(err)
	}
	return store.Agent{Project: testProject, Name: name}
}

// TestTheBoardReportsTheObserversFill: the context figures and the model on the board come from the
// watchdog's sample, not from a read the render takes. Read per agent per render, they parsed a
// session transcript per row — and there is one render per connected client, per refresh, plus one
// after every mutation.
func TestTheBoardReportsTheObserversFill(t *testing.T) {
	h := newHub(t)
	w := stillWatchdog(t, h)
	a := observedAgent(t, h, "dvalin", "claude-sonnet-5")

	w.record(a, true, 0, agent.Observation{Runtime: "idle", Digest: "d1"})
	w.recordFill(a, fill{tokens: 120_000, window: 200_000, model: "claude-opus-5"})

	view := onlyAgent(t, h)
	if view.ContextTokens != 120_000 || view.ContextWindow != 200_000 {
		t.Errorf("board reports %d/%d tokens, want the sampled 120000/200000",
			view.ContextTokens, view.ContextWindow)
	}
	// A model switched by hand inside Claude Code shows up in the transcript, never in the roster, so
	// the detected one wins while the agent is up.
	if view.Model != "claude-opus-5" {
		t.Errorf("board reports model %q, want the detected claude-opus-5", view.Model)
	}
}

// TestTheSweepSamplesTheTranscript is the other half of the invariant, and the half carrying all the
// wiring: the board reports figures it does not take, which only works because the sweep takes them.
// Every other case here hand-feeds recordFill, so without this one the sample could be deleted
// outright and the suite would stay green while every agent read zero tokens for ever.
func TestTheSweepSamplesTheTranscript(t *testing.T) {
	tokens := 640_000
	agentport.Use(fakeAgent{tokens: &tokens})
	t.Cleanup(func() { agentport.Use(fakeAgent{}) })

	h := newHub(t)
	w := stillWatchdog(t, h)
	observedAgent(t, h, "dvalin", "claude-sonnet-5")

	// One sweep, probes included — the beat the transcript rides. There is no container runtime here,
	// so the liveness half fails and records the agent down, which is what makes the ordering matter:
	// the fill is folded in after that entry exists (-> watchdog.sweep).
	w.sweep(true)

	l, seen := w.get(testProject, "dvalin")
	if !seen {
		t.Fatal("the sweep recorded no observation at all")
	}
	if l.tokens != 640_000 || l.window != 1_000_000 {
		t.Errorf("the sweep sampled %d/%d tokens, want the transcript's 640000/1000000", l.tokens, l.window)
	}
	if l.model != "claude-opus-5" {
		t.Errorf("the sweep sampled model %q, want the transcript's claude-opus-5", l.model)
	}
	// And it reaches the board, which is the only reason it is sampled at all.
	if view := onlyAgent(t, h); view.ContextTokens != 640_000 || view.ContextWindow != 1_000_000 {
		t.Errorf("board reports %d/%d tokens, want the sweep's 640000/1000000",
			view.ContextTokens, view.ContextWindow)
	}
}

// TestADownAgentShowsTheModelItWasStartedOn: there is no live session to detect one off, so the
// roster's own record is all there is — and it must not be overridden by whatever the last transcript
// happened to say.
func TestADownAgentShowsTheModelItWasStartedOn(t *testing.T) {
	h := newHub(t)
	w := stillWatchdog(t, h)
	a := observedAgent(t, h, "dvalin", "claude-sonnet-5")

	w.recordFill(a, fill{tokens: 1_000, window: 200_000, model: "claude-opus-5"})
	for i := 0; i <= downStrikes; i++ {
		w.record(a, false, 0, agent.Observation{})
	}

	if view := onlyAgent(t, h); view.Model != "claude-sonnet-5" {
		t.Errorf("a down agent reports model %q, want the roster's claude-sonnet-5", view.Model)
	}
}

// TestTheBoardStatusIgnoresFullness: fullness no longer waits on the user (an idle ask clears and
// reassigns a full worker automatically), so the board's status word never reads anything but
// "idle" for one, however close to its window it sits — only the raw ContextTokens says so.
func TestTheBoardStatusIgnoresFullness(t *testing.T) {
	h := newHub(t)
	w := stillWatchdog(t, h)
	a := observedAgent(t, h, "dvalin", "")
	w.record(a, true, 0, agent.Observation{Runtime: "idle", Digest: "d1"})

	w.recordFill(a, fill{tokens: 120_000, window: 200_000})
	if view := onlyAgent(t, h); view.Status != "idle" {
		t.Errorf("at 60%% of its window an agent is idle, not %q", view.Status)
	}
	w.recordFill(a, fill{tokens: 190_000, window: 200_000})
	view := onlyAgent(t, h)
	if view.Status != "idle" {
		t.Errorf("at 95%% of its window an idle agent still reads %q, want idle", view.Status)
	}
	if view.ContextTokens != 190_000 {
		t.Errorf("ContextTokens = %d, want 190000 — the raw fill still carries the figure", view.ContextTokens)
	}
}

// TestAFillSurvivesALivenessReading: the two readings are taken on different beats, so folding one in
// must not blank the other — a board whose context column emptied every sweep is what that looks like.
func TestAFillSurvivesALivenessReading(t *testing.T) {
	h := newHub(t)
	w := stillWatchdog(t, h)
	a := observedAgent(t, h, "dvalin", "")

	w.record(a, true, 0, agent.Observation{Runtime: "idle", Digest: "d1"})
	w.recordFill(a, fill{tokens: 90_000, window: 200_000, model: "claude-opus-5"})
	w.record(a, true, 1, agent.Observation{Runtime: "working", Digest: "d2"})

	l, _ := w.get(testProject, "dvalin")
	if l.tokens != 90_000 || l.model != "claude-opus-5" {
		t.Errorf("after a liveness reading the fill is %d/%q, want the last sample 90000/claude-opus-5",
			l.tokens, l.model)
	}
}

// TestAFillNeedsAnObservationFirst: an agent nothing has looked at must read "unknown", so a fill
// alone must not create the entry that would make it read "observed, and down".
func TestAFillNeedsAnObservationFirst(t *testing.T) {
	h := newHub(t)
	w := stillWatchdog(t, h)
	a := observedAgent(t, h, "dvalin", "")

	w.recordFill(a, fill{tokens: 90_000, window: 200_000})
	if _, seen := w.get(testProject, "dvalin"); seen {
		t.Fatal("a transcript reading is not an observation of the agent")
	}
	if view := onlyAgent(t, h); view.Status != "unknown" {
		t.Errorf("an unobserved agent reads %q, want unknown", view.Status)
	}
}

// onlyAgent is the board's single agent row, so each case asserts against what a UI would render.
func onlyAgent(t *testing.T, h *Hub) AgentView {
	t.Helper()
	board, err := h.State("")
	if err != nil {
		t.Fatalf("board read: %v", err)
	}
	if len(board.Agents) != 1 {
		t.Fatalf("board carries %d agents, want the one registered", len(board.Agents))
	}
	return board.Agents[0]
}
