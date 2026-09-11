package fleet

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/world/store"

	_ "modernc.org/sqlite"
)

// podFleet seeds a project with whatever agents the test wants, handing back the database path too
// — the idle reclaim is a DWELL now, a fact on the state row rather than a memo the hub loses when
// it restarts, so a test that needs one to have passed ages the row (-> standIn).
func podFleet(t *testing.T, d *flowtest.Hub, agents ...store.Agent) (*Engine, *store.ProjectStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("repo", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	for _, a := range agents {
		if err := ps.PutAgent(a); err != nil {
			t.Fatal(err)
		}
		if a.Stopped {
			// A reclaimed pod is not there. Said to the observer as well as written on the row: the
			// two disagreeing is a world the hub never produces, and half these questions turn on it.
			if d.DownAgents == nil {
				d.DownAgents = map[string]bool{}
			}
			d.DownAgents[a.Name] = true
		}
	}
	return newEngine(t, st, d), ps, path
}

// standIn puts an agent in a state and ages when it got there, so a dwell a test needs to be past is
// reached without waiting for it. The stamp is written from outside rather than through a store
// method that exists for nobody: how long a subject has stood somewhere is the hub's to record, and
// a setup that needs a different answer is arranging data, not calling the hub.
func standIn(t *testing.T, ps *store.ProjectStore, path, agent, state string, since time.Duration) {
	t.Helper()
	flowtest.Place(t, ps, store.AgentState{Agent: agent, Phase: state})
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE agent_state SET phase_since=? WHERE agent=?`,
		time.Now().Add(-since).UTC().Format(time.RFC3339), agent); err != nil {
		t.Fatal(err)
	}
}

// waiting puts one claimable task in the backlog. Through the owned table and a sync, as the hub
// does: the owned row is the source, and the cached read model is what every claim reads.
func waiting(t *testing.T, e *Engine, ps *store.ProjectStore) {
	t.Helper()
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-1", Title: "a task", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := e.taskAct().SyncTasks("repo"); err != nil {
		t.Fatal(err)
	}
}

// TestAnIdleWorkerHasItsPodReclaimed: idleness alone triggers it, never memory pressure — a stop
// preserves the session, so reclaiming costs only the next start's latency.
func TestAnIdleWorkerHasItsPodReclaimed(t *testing.T) {
	d := &flowtest.Hub{Root: t.TempDir()}
	e, ps, path := podFleet(t, d, store.Agent{Name: "durin", Role: "worker", Workspace: ".worktrees/durin"})
	standIn(t, ps, path, "durin", worker.Idle, flow.IdleStopThreshold+time.Minute)

	e.Look("repo", "durin")

	if len(d.Stopped) != 1 || d.Stopped[0] != "durin" {
		t.Errorf("stopped = %v, want durin's pod reclaimed after %s idle", d.Stopped, flow.IdleStopThreshold)
	}
}

// TestAWorkerIdleForLessThanTheThresholdKeepsItsPod: the dwell is the whole rule, so just under it
// must leave the pod alone.
func TestAWorkerIdleForLessThanTheThresholdKeepsItsPod(t *testing.T) {
	d := &flowtest.Hub{Root: t.TempDir()}
	e, ps, path := podFleet(t, d, store.Agent{Name: "durin", Role: "worker", Workspace: ".worktrees/durin"})
	standIn(t, ps, path, "durin", worker.Idle, flow.IdleStopThreshold-time.Minute)

	e.Look("repo", "durin")

	if len(d.Stopped) != 0 {
		t.Errorf("stopped = %v, want none — it has not been idle long enough", d.Stopped)
	}
}

// TestARetiredWorkerIsNotReclaimed: retirement exempts every automatic behaviour, and the wind-down
// is the user's to finish rather than the hub's. The rule is the surface's, asked rather than
// restated (-> situation.Surface.Reclaim).
func TestARetiredWorkerIsNotReclaimed(t *testing.T) {
	d := &flowtest.Hub{Root: t.TempDir()}
	e, ps, path := podFleet(t, d, store.Agent{Name: "durin", Role: "worker", Workspace: ".worktrees/durin", Retired: true})
	standIn(t, ps, path, "durin", worker.Retired, flow.IdleStopThreshold+time.Minute)

	e.Look("repo", "durin")

	if len(d.Stopped) != 0 {
		t.Errorf("stopped = %v, want none — a retired agent's wind-down is the user's", d.Stopped)
	}
}

// TestWaitingWorkStartsOneStoppedWorker: waking is the missing half of reclaiming, and exactly one
// pod comes back — the machine looks at one agent at a time, so without a fleet-shaped answer every
// stopped worker would start itself for the same task.
func TestWaitingWorkStartsOneStoppedWorker(t *testing.T) {
	d := &flowtest.Hub{Root: t.TempDir()}
	e, ps, _ := podFleet(t, d,
		store.Agent{Name: "durin", Role: "worker", Workspace: ".worktrees/durin", Stopped: true},
		store.Agent{Name: "nori", Role: "worker", Workspace: ".worktrees/nori", Stopped: true})
	waiting(t, e, ps)

	e.LookProject("repo")

	if len(d.Started) != 1 {
		t.Fatalf("started = %v, want exactly one — a second is woken only if a second row is still waiting", d.Started)
	}
}

// TestAStoppedWorkerStaysDownWithNothingWaiting: nothing to wake anyone for.
func TestAStoppedWorkerStaysDownWithNothingWaiting(t *testing.T) {
	d := &flowtest.Hub{Root: t.TempDir()}
	e, _, _ := podFleet(t, d, store.Agent{Name: "durin", Role: "worker", Workspace: ".worktrees/durin", Stopped: true})

	e.LookProject("repo")

	if len(d.Started) != 0 {
		t.Errorf("started = %v, want none — the backlog is empty", d.Started)
	}
}

// TestALiveIdleWorkerIsLeftToClaimItItself: a worker that is up and empty-handed will take the task
// on its own next pass, so bringing a second pod back for it would spend a launch for nothing.
func TestALiveIdleWorkerIsLeftToClaimItItself(t *testing.T) {
	d := &flowtest.Hub{Root: t.TempDir()}
	e, ps, _ := podFleet(t, d,
		store.Agent{Name: "durin", Role: "worker", Workspace: ".worktrees/durin"},
		store.Agent{Name: "nori", Role: "worker", Workspace: ".worktrees/nori", Stopped: true})
	waiting(t, e, ps)

	e.Look("repo", "nori")

	if len(d.Started) != 0 {
		t.Errorf("started = %v, want none — durin is up and empty-handed, and will claim it itself", d.Started)
	}
}

// TestAWaitingTaskNeverWakesAReviewer: one role's queue must never wake the other's.
func TestAWaitingTaskNeverWakesAReviewer(t *testing.T) {
	d := &flowtest.Hub{Root: t.TempDir()}
	e, ps, _ := podFleet(t, d, store.Agent{Name: "fili", Role: "reviewer", Workspace: ".worktrees/fili", Stopped: true})
	waiting(t, e, ps)

	e.LookProject("repo")

	if len(d.Started) != 0 {
		t.Errorf("started = %v, want none — an open task is a worker's demand signal", d.Started)
	}
}

// TestAWaitingReviewNeverWakesAWorker is the same guard the other way round.
func TestAWaitingReviewNeverWakesAWorker(t *testing.T) {
	d := &flowtest.Hub{Root: t.TempDir()}
	e, ps, _ := podFleet(t, d, store.Agent{Name: "durin", Role: "worker", Workspace: ".worktrees/durin", Stopped: true})
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "td-1", Agent: "bombur", Branch: "sd-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	flowtest.FileReview(t, ps, "pr-1")

	e.LookProject("repo")

	if len(d.Started) != 0 {
		t.Errorf("started = %v, want none — an unclaimed review is a reviewer's demand signal", d.Started)
	}
}

// TestAHumansStopIsCarriedOutByTheMachine: a request rather than a flag, because the agent may
// already be in whatever state a flag would claim — and the machine is what acts on it.
func TestAHumansStopIsCarriedOutByTheMachine(t *testing.T) {
	d := &flowtest.Hub{Root: t.TempDir()}
	e, ps, _ := podFleet(t, d, store.Agent{Name: "durin", Role: "worker", Workspace: ".worktrees/durin"})

	if err := e.roleAct().AskStop("repo", "durin"); err != nil {
		t.Fatal(err)
	}
	if ag, _, _ := ps.GetAgent("durin"); !ag.StopAsked {
		t.Error("the ask must be recorded, so a hub that restarts before acting still knows it was made")
	}

	e.Look("repo", "durin")

	if len(d.Stopped) != 1 || d.Stopped[0] != "durin" {
		t.Errorf("stopped = %v, want durin's pod taken back", d.Stopped)
	}
	if ag, _, _ := ps.GetAgent("durin"); ag.StopAsked {
		t.Error("a request left standing brings the agent back into the state that answers it, for ever")
	}
}
