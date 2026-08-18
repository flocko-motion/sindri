package agent

import (
	"testing"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/agent/claude"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
)

// sleepFixture wires a service over a fake tmux backend, with one running worker holding nothing.
func sleepFixture(t *testing.T) (*Service, *fakeRuntime) {
	t.Helper()
	_, st := newService(t)
	s := New(st, tellDeps{}, nil)
	if err := st.For("proj").PutAgent(store.Agent{Name: "durin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := st.For("proj").SetState(store.AgentState{Agent: "durin", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	f := &fakeRuntime{pane: idlePane}
	container.Use(f)
	agentport.Use(claude.New())
	t.Cleanup(func() {
		container.UseDefault()
		agentport.Use(unreadablePane{})
	})
	forgetObservations()
	t.Cleanup(forgetObservations)
	return s, f
}

// TestIdleSinceOrMarkTracksASpell: the first tick a spell is seen it is never due, however far in
// the future "now" is by the time the next tick asks — the spell starts at the first observation.
func TestIdleSinceOrMarkTracksASpell(t *testing.T) {
	key := lcKey{"proj", "durin"}
	t.Cleanup(func() { forgetIdleSince(key) })
	base := time.Now()
	if _, due := idleSinceOrMark(key, base); due {
		t.Error("the first tick must never be due — nothing has been idle long enough yet")
	}
	if _, due := idleSinceOrMark(key, base.Add(IdleStopThreshold-time.Second)); due {
		t.Error("due a second early")
	}
	since, due := idleSinceOrMark(key, base.Add(IdleStopThreshold))
	if !due || !since.Equal(base) {
		t.Errorf("due=%v since=%v, want due at exactly the threshold, since = the first tick", due, since)
	}
}

// TestForgetIdleSinceStartsANewSpell: work arriving and idleness returning is a new spell, not a
// resumed one — the old start must not survive a forget.
func TestForgetIdleSinceStartsANewSpell(t *testing.T) {
	key := lcKey{"proj", "durin"}
	base := time.Now()
	idleSinceOrMark(key, base)
	forgetIdleSince(key)
	if _, due := idleSinceOrMark(key, base.Add(IdleStopThreshold)); due {
		t.Error("forgetting must restart the spell — the old start must not survive")
	}
	t.Cleanup(func() { forgetIdleSince(key) })
}

// TestHoldsNothingChecksEveryKindOfWork: a task, a held feature, an owed review, or an open
// escalation each mean the worker is mid-something, not idle.
func TestHoldsNothingChecksEveryKindOfWork(t *testing.T) {
	s, _ := sleepFixture(t)
	if empty, err := s.HoldsNothing("proj", "durin", "worker"); err != nil || !empty {
		t.Fatalf("a fresh idle worker should hold nothing: empty=%v err=%v", empty, err)
	}
	ps := s.store.For("proj")
	for _, c := range []struct {
		name  string
		setup func() error
	}{
		{"task", func() error { return ps.SetState(store.AgentState{Agent: "durin", Task: "td-1", Phase: "working"}) }},
		{"feature", func() error {
			return ps.SetState(store.AgentState{Agent: "durin", Container: "sd-epic", Phase: "idle"})
		}},
		{"escalation", func() error {
			if err := ps.SetState(store.AgentState{Agent: "durin", Phase: "idle"}); err != nil {
				return err
			}
			return ps.SetEscalation("durin", "which approach?")
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := c.setup(); err != nil {
				t.Fatal(err)
			}
			if empty, err := s.HoldsNothing("proj", "durin", "worker"); err != nil || empty {
				t.Errorf("holding %s should not read as holding nothing: empty=%v err=%v", c.name, empty, err)
			}
		})
	}
}

// TestHoldsNothingExcludesCoauthor: a coauthor's session is the user's own seat, never idle
// capacity — checked by role alone, before any state is even read.
func TestHoldsNothingExcludesCoauthor(t *testing.T) {
	s, _ := sleepFixture(t)
	if empty, err := s.HoldsNothing("proj", "durin", "coauthor"); err != nil || empty {
		t.Errorf("a coauthor must never read as holding nothing: empty=%v err=%v", empty, err)
	}
}

// TestFireIdleStopsReclaimsAPastDueSpell runs the real thing: a spell seeded as already past
// threshold (rather than waiting for real time to pass) must be stopped and the record cleared.
func TestFireIdleStopsReclaimsAPastDueSpell(t *testing.T) {
	s, f := sleepFixture(t)
	key := lcKey{"proj", "durin"}
	idleSinceOrMark(key, time.Now().Add(-IdleStopThreshold-time.Minute))
	t.Cleanup(func() { forgetIdleSince(key) })

	s.FireIdleStops("proj")

	if len(f.removed) == 0 {
		t.Error("a worker idle past the threshold was never stopped")
	}
	a, _, err := s.store.For("proj").GetAgent("durin")
	if err != nil {
		t.Fatal(err)
	}
	if !a.Stopped {
		t.Error("Stopped must be set once the idle sweep reclaims the pod")
	}
}

// TestFireIdleStopsLeavesARetiredWorkerAlone: retirement exempts every automatic behaviour,
// including reclaiming a pod nobody is using — the temptation memory pressure would make worse.
func TestFireIdleStopsLeavesARetiredWorkerAlone(t *testing.T) {
	s, f := sleepFixture(t)
	ps := s.store.For("proj")
	a, _, err := ps.GetAgent("durin")
	if err != nil {
		t.Fatal(err)
	}
	a.Retired = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	key := lcKey{"proj", "durin"}
	idleSinceOrMark(key, time.Now().Add(-IdleStopThreshold-time.Minute))
	t.Cleanup(func() { forgetIdleSince(key) })

	s.FireIdleStops("proj")

	if len(f.removed) != 0 {
		t.Error("a retired worker was stopped anyway")
	}
}

// TestFireIdleStartsWakesAStoppedWorkerForWaitingWork: the demand signal is OpenLeaves, the same
// one the assignment gate reads — a stopped worker in a repo with open, claimable work is started.
func TestFireIdleStartsWakesAStoppedWorkerForWaitingWork(t *testing.T) {
	s, _ := sleepFixture(t)
	ps := s.store.For("proj")
	a, _, err := ps.GetAgent("durin")
	if err != nil {
		t.Fatal(err)
	}
	a.Stopped = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-1", Title: "waiting", Status: "open", Priority: "P2"}); err != nil {
		t.Fatal(err)
	}
	// OpenLeaves reads the synced cache table, not owned_tasks directly — a real sync keeps them in
	// step; here nothing syncs, so the row has to be placed by hand.
	if err := ps.UpsertTask(store.Task{ID: "td-1", Status: "open", Priority: "P2", Type: "task"}); err != nil {
		t.Fatal(err)
	}

	s.FireIdleStarts("proj")

	// Launch fails at its own pre-flight (fakeRuntime.Check refuses), but Stopped must already have
	// been cleared by the time it got there — that happens before Check ever runs.
	got, _, err := ps.GetAgent("durin")
	if err != nil {
		t.Fatal(err)
	}
	if got.Stopped {
		t.Error("waking a stopped worker must clear Stopped even if the relaunch itself later fails")
	}
}

// TestFireIdleStartsDoesNothingWithoutWaitingWork: nothing to wake anyone for.
func TestFireIdleStartsDoesNothingWithoutWaitingWork(t *testing.T) {
	s, _ := sleepFixture(t)
	ps := s.store.For("proj")
	a, _, err := ps.GetAgent("durin")
	if err != nil {
		t.Fatal(err)
	}
	a.Stopped = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}

	s.FireIdleStarts("proj")

	got, _, err := ps.GetAgent("durin")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Stopped {
		t.Error("a worker was woken with no waiting work to justify it")
	}
}

// TestFireIdleStartsLeavesAnAlreadyIdleWorkerToClaimItItself: a live idle worker will claim the
// waiting work within its own next poll — waking a stopped one too would pay for a container
// nobody needed.
func TestFireIdleStartsLeavesAnAlreadyIdleWorkerToClaimItItself(t *testing.T) {
	s, _ := sleepFixture(t)
	ps := s.store.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "nori", Role: "worker", Stopped: true}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-1", Title: "waiting", Status: "open", Priority: "P2"}); err != nil {
		t.Fatal(err)
	}
	// OpenLeaves reads the synced cache table, not owned_tasks directly — a real sync keeps them in
	// step; here nothing syncs, so the row has to be placed by hand.
	if err := ps.UpsertTask(store.Task{ID: "td-1", Status: "open", Priority: "P2", Type: "task"}); err != nil {
		t.Fatal(err)
	}

	s.FireIdleStarts("proj") // "durin" is live and idle; "nori" is stopped

	got, _, err := ps.GetAgent("nori")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Stopped {
		t.Error("a stopped worker was woken even though a live idle one could claim the work itself")
	}
}

// unclaimedReview seeds a repo's PR at the status UnclaimedReview requires (RequestReview's own
// rule: "the PR has to be open for anyone to be handed it") with one review nobody has claimed yet.
func unclaimedReview(t *testing.T, ps *store.ProjectStore, prID string) {
	t.Helper()
	if err := ps.PutPR(store.PR{ID: prID, Task: "td-1", Agent: "wrk", Branch: "td-1", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddReview(prID, "check it"); err != nil {
		t.Fatal(err)
	}
}

// TestFireIdleStartsWakesAStoppedReviewerForAWaitingReview: the demand signal for a reviewer is an
// unclaimed review, not OpenLeaves — a stopped reviewer with a PR nobody has claimed is started.
func TestFireIdleStartsWakesAStoppedReviewerForAWaitingReview(t *testing.T) {
	s, _ := sleepFixture(t)
	ps := s.store.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "rune", Role: "reviewer", Stopped: true}); err != nil {
		t.Fatal(err)
	}
	unclaimedReview(t, ps, "pr-1")

	s.FireIdleStarts("proj")

	got, _, err := ps.GetAgent("rune")
	if err != nil {
		t.Fatal(err)
	}
	if got.Stopped {
		t.Error("a stopped reviewer with an unclaimed review waiting was never woken")
	}
}

// TestFireIdleStartsDoesNotWakeAStoppedReviewerForAWaitingTask: an open task is a worker's demand
// signal, not a reviewer's — the exact bug report this guards: a stopped reviewer must never be
// mistaken for a fit answer to a task nobody but a worker can claim.
func TestFireIdleStartsDoesNotWakeAStoppedReviewerForAWaitingTask(t *testing.T) {
	s, _ := sleepFixture(t)
	ps := s.store.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "rune", Role: "reviewer", Stopped: true}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-1", Title: "waiting", Status: "open", Priority: "P2"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-1", Status: "open", Priority: "P2", Type: "task"}); err != nil {
		t.Fatal(err)
	}

	s.FireIdleStarts("proj") // no worker in the roster at all — nothing can claim td-1

	got, _, err := ps.GetAgent("rune")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Stopped {
		t.Error("a stopped reviewer was woken for an open task, which it cannot claim")
	}
}

// TestFireIdleStartsDoesNotWakeAStoppedWorkerForAWaitingReview is the same guard the other
// direction: an unclaimed review is a reviewer's demand signal, not a worker's.
func TestFireIdleStartsDoesNotWakeAStoppedWorkerForAWaitingReview(t *testing.T) {
	s, _ := sleepFixture(t)
	ps := s.store.For("proj")
	a, _, err := ps.GetAgent("durin")
	if err != nil {
		t.Fatal(err)
	}
	a.Stopped = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	unclaimedReview(t, ps, "pr-1")

	s.FireIdleStarts("proj") // no reviewer in the roster at all — nothing can claim pr-1's review

	got, _, err := ps.GetAgent("durin")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Stopped {
		t.Error("a stopped worker was woken for an unclaimed review, which it cannot claim")
	}
}
