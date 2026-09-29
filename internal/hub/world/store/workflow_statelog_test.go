package store

import (
	"fmt"
	"path/filepath"
	"testing"
)

// SetHolding must log every call under the caller's own reason and detail — the whole point of
// requiring them: a change nobody can say why for should not be writable.
func TestSetHoldingLogsTheReason(t *testing.T) {
	p := openTmpProject(t)
	if err := p.SetHolding("brokkr", "td-1", "", "", ReasonClaimed, "claimed td-1"); err != nil {
		t.Fatal(err)
	}
	log, err := p.StateLog("brokkr", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "claimed td-1 -> task=td-1"
	if len(log) != 1 || log[0].Reason != string(ReasonClaimed) || log[0].Detail != want {
		t.Fatalf("want one claimed/%q row, got %+v", want, log)
	}
}

// SetPhase must log too — the field-wise mutator is not exempt from the same requirement SetState
// carries.
func TestSetPhaseLogsTheReason(t *testing.T) {
	p := openTmpProject(t)
	if err := place(t, p, "brokkr", "", "", "", "working", ReasonClaimed, "setup"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetPhase("brokkr", "resolving", ReasonAdvanced, "conflict"); err != nil {
		t.Fatal(err)
	}
	log, err := p.StateLog("brokkr", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "conflict -> phase=resolving"
	if len(log) != 3 || log[0].Reason != string(ReasonAdvanced) || log[0].Detail != want {
		t.Fatalf("want the resolving/%q row newest-first, got %+v", want, log)
	}
}

// SetHolding and SetPhase both append what they wrote to the detail, so a row answers "what was it"
// as well as "why" — the class of gap that left a state_log row unable to show a dropped container.
// Each says only what IT writes, which is how the two writers read apart on the record.
func TestLogStateDetailCarriesWhatChanged(t *testing.T) {
	p := openTmpProject(t)
	if err := place(t, p, "brokkr", "td-1", "", "td-feature", "working", ReasonClaimed, "claimed subtask"); err != nil {
		t.Fatal(err)
	}
	log, err := p.StateLog("brokkr", 0)
	if err != nil {
		t.Fatal(err)
	}
	held, stood := "claimed subtask -> task=td-1 container=td-feature", "claimed subtask -> phase=working"
	if len(log) != 2 || log[1].Detail != held || log[0].Detail != stood {
		t.Fatalf("want %q then %q, got %+v", held, stood, log)
	}
}

// StateLog is newest first, and its own limit caps independently of stateLogCap.
func TestStateLogNewestFirstAndLimit(t *testing.T) {
	p := openTmpProject(t)
	for i := 0; i < 5; i++ {
		// Distinct: a row repeating the one before it is deliberately not appended (-> LogPass).
		if err := p.LogState("brokkr", ReasonAdvanced, fmt.Sprintf("step %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	all, err := p.StateLog("brokkr", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Fatalf("want 5 rows, got %d", len(all))
	}
	for i := 0; i < len(all)-1; i++ {
		if all[i].ID < all[i+1].ID {
			t.Fatalf("not newest-first: %+v", all)
		}
	}
	capped, err := p.StateLog("brokkr", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(capped) != 2 || capped[0].ID != all[0].ID {
		t.Fatalf("limit=2 should keep the 2 newest, got %+v", capped)
	}
}

// state_log is capped per agent at write time — a hub up for weeks must not hold weeks of a
// flickering derived word.
func TestStateLogCapsPerAgent(t *testing.T) {
	p := openTmpProject(t)
	for i := 0; i < stateLogCap+10; i++ {
		if err := p.LogState("brokkr", ReasonStatus, fmt.Sprintf("tick %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	all, err := p.StateLog("brokkr", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != stateLogCap {
		t.Errorf("want exactly %d rows kept, got %d", stateLogCap, len(all))
	}
	// A different agent's rows are untouched by another agent's trim.
	if err := p.LogState("eitri", ReasonStatus, "tick"); err != nil {
		t.Fatal(err)
	}
	eitri, err := p.StateLog("eitri", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(eitri) != 1 {
		t.Errorf("another agent's trim should not touch eitri's own row, got %d", len(eitri))
	}
}

// The record SURVIVES a restart, which is the whole reason it is worth keeping: the interesting
// failures span one, and a hub that restarts on install was a hub that forgot its own evidence.
func TestStateLogSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sindri.db")
	first, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := first.For("repo").LogState("brokkr", ReasonStatus, "before restart"); err != nil {
		t.Fatalf("LogState: %v", err)
	}
	first.Close()
	again, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer again.Close()
	log, err := again.For("repo").StateLog("brokkr", 0)
	if err != nil {
		t.Fatalf("StateLog: %v", err)
	}
	if len(log) != 1 || log[0].Detail != "before restart" {
		t.Errorf("the record must survive a restart, got %+v", log)
	}
}

// A reconcile pass's steps share one correlation id, so an agent's story is ONE query rather than
// several logs aligned by eye — which is what every hub bug chased this week needed by hand.
func TestAPassIsOneQuery(t *testing.T) {
	p := openTmpProject(t)
	for _, step := range []string{"decided", "started", "finished"} {
		if err := p.LogPass("brokkr", step, "p-00002a", step+" something"); err != nil {
			t.Fatalf("LogPass: %v", err)
		}
	}
	log, err := p.StateLog("brokkr", 0)
	if err != nil {
		t.Fatalf("StateLog: %v", err)
	}
	if len(log) != 3 {
		t.Fatalf("want three rows, got %+v", log)
	}
	for _, e := range log {
		if e.Pass != "p-00002a" {
			t.Errorf("every line of one pass must carry its id, got %+v", e)
		}
	}
}

// Telemetry must not fail the write it observes: SetState commits the agent_state row before it
// ever tries to log, so a broken state_log insert must not be reported as though the state write
// itself failed — a caller that gets nil back must be able to trust the state landed.
func TestSetStateSucceedsEvenWhenLogStateFails(t *testing.T) {
	p := openTmpProject(t)
	if _, err := p.s.db.Exec(`DROP TABLE state_log`); err != nil {
		t.Fatal(err)
	}
	if err := place(t, p, "brokkr", "td-1", "", "", "working", ReasonClaimed, "test"); err != nil {
		t.Fatalf("SetState must succeed even though state_log is gone, got: %v", err)
	}
	got, err := p.GetState("brokkr")
	if err != nil {
		t.Fatal(err)
	}
	if got.Task != "td-1" || got.Phase != "working" {
		t.Errorf("the state itself must still have landed, got %+v", got)
	}
}

// Same guarantee for SetPhase, whose OWN pre-existing contract ("errors rather than quietly writing
// nothing", pinned by TestSetPhaseErrorsRatherThanNoOpOnAnUnknownAgent) makes this one sharper: an
// error here must mean the phase genuinely did not change, never "changed fine, telemetry failed".
func TestSetPhaseSucceedsEvenWhenLogStateFails(t *testing.T) {
	p := openTmpProject(t)
	if err := place(t, p, "brokkr", "", "", "", "working", ReasonClaimed, "setup"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.s.db.Exec(`DROP TABLE state_log`); err != nil {
		t.Fatal(err)
	}
	if err := p.SetPhase("brokkr", "resolving", ReasonAdvanced, "test"); err != nil {
		t.Fatalf("SetPhase must succeed even though state_log is gone, got: %v", err)
	}
	got, err := p.GetState("brokkr")
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != "resolving" {
		t.Errorf("the phase itself must still have landed, got %q", got.Phase)
	}
}

// ClearEscalation's row must say WHAT was cleared, not just that something was — the same "why but
// not what" gap SetState/SetPhase closed via whatChanged; SetEscalation already answers it (the
// question IS the detail), which made ClearEscalation's silence the odd one out.
func TestClearEscalationLogsWhatWasCleared(t *testing.T) {
	p := openTmpProject(t)
	if err := place(t, p, "brokkr", "", "", "", "working", ReasonClaimed, "setup"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetEscalation("brokkr", "should this touch prod config?"); err != nil {
		t.Fatal(err)
	}
	if err := p.ClearEscalation("brokkr"); err != nil {
		t.Fatal(err)
	}
	log, err := p.StateLog("brokkr", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "escalation cleared: should this touch prod config?"
	if len(log) == 0 || log[0].Detail != want {
		t.Fatalf("want the clear row to name what it cleared (%q), got %+v", want, log)
	}
}

// TestPhaseSinceStampsOnlyARealChange: a long-running action IS a phase, and the watcher over one
// measures its age — a re-write of the same phase that restamped it would hide an agent stuck there.
func TestPhaseSinceStampsOnlyARealChange(t *testing.T) {
	p := openTmpProject(t)
	if err := place(t, p, "brokkr", "", "", "", "clearing", ReasonAdvanced, "clearing"); err != nil {
		t.Fatalf("SetState: %v", err)
	}
	first, err := p.GetState("brokkr")
	if err != nil || first.PhaseSince == "" {
		t.Fatalf("PhaseSince must be stamped on a phase change, got %+v (err %v)", first, err)
	}
	if err := p.SetPhase("brokkr", "clearing", ReasonAdvanced, "still clearing"); err != nil {
		t.Fatalf("SetPhase: %v", err)
	}
	again, _ := p.GetState("brokkr")
	if again.PhaseSince != first.PhaseSince {
		t.Errorf("re-writing the same phase must not restamp it: %q -> %q", first.PhaseSince, again.PhaseSince)
	}
	if err := p.SetPhase("brokkr", "working", ReasonAdvanced, "cleared"); err != nil {
		t.Fatalf("SetPhase: %v", err)
	}
	moved, _ := p.GetState("brokkr")
	if moved.PhaseSince == "" {
		t.Errorf("a real phase change must stamp PhaseSince, got %+v", moved)
	}
}

// TestPassesCannotEvictHistory is thrain's five minutes: a prod re-running every two seconds pushed
// its whole claim chain past a shared cap, leaving three rows of history in five hundred. The two
// classes are trimmed against their own budgets, so a burst of passes costs the agent nothing.
func TestPassesCannotEvictHistory(t *testing.T) {
	p := openTmpProject(t)
	if err := p.LogState("brokkr", ReasonClaimed, "claimed sd-1"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < passLogCap*3; i++ {
		if err := p.LogPass("brokkr", "started", "p-1", fmt.Sprintf("prod %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	all, err := p.StateLog("brokkr", 0)
	if err != nil {
		t.Fatal(err)
	}
	var history, passes int
	for _, e := range all {
		if e.Reason == string(ReasonClaimed) {
			history++
		}
		if e.Reason == "started" {
			passes++
		}
	}
	if history != 1 {
		t.Errorf("the claim must survive any number of passes, got %d rows of history in %d", history, len(all))
	}
	if passes != passLogCap {
		t.Errorf("passes should be held to their own budget, got %d want %d", passes, passLogCap)
	}
}

// TestARepeatedRowIsNotAppended: a spell of the same thing happening says what one row says, and
// repeating it is how a history comes to hold nothing but the last few minutes of a loop.
func TestARepeatedRowIsNotAppended(t *testing.T) {
	p := openTmpProject(t)
	for i := 0; i < 50; i++ {
		if err := p.LogPass("brokkr", "outcome", "p-1", "prod: nothing"); err != nil {
			t.Fatal(err)
		}
	}
	all, err := p.StateLog("brokkr", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("fifty identical rows should read as one, got %d", len(all))
	}
	// A DIFFERENT row still lands, and the one after that repeats nothing.
	if err := p.LogPass("brokkr", "outcome", "p-1", "prod: done"); err != nil {
		t.Fatal(err)
	}
	if all, _ = p.StateLog("brokkr", 0); len(all) != 2 {
		t.Errorf("a row that says something new must be appended, got %d", len(all))
	}
}
