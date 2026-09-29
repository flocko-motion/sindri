package machine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// failing is the action the test runs: it names where the subject belongs AND reports why.
var failing = &Action{Name: "failing", Outcomes: []Outcome{done, failed}}

var (
	done   = Outcome{Name: "done"}
	failed = Outcome{Name: "failed"}
)

// never is the condition exit every acting state must declare; it holds for nothing.
var never = Condition[int]{Name: "never", Within: time.Minute, Holds: func(int) bool { return false }}

// TestAnActionsOutcomeLandsEvenWhenItAlsoFailed holds the fault that parked a worker in
// `worker/assigning` for an hour: the claim failed on a detached HEAD, said `held -> idle`, and the
// engine dropped that answer because an error came with it. The subject stayed in the acting state
// and the action was restarted on every poll, for ever.
func TestAnActionsOutcomeLandsEvenWhenItAlsoFailed(t *testing.T) {
	m, at := actingMachine(t, func(context.Context, int) (Outcome, error) {
		return failed, errors.New("the repo is on a detached HEAD")
	})
	m.Look("subject")
	if got := at.get(); got != "stopped" {
		t.Fatalf("state = %q, want %q — a named outcome moves the subject whatever else the action reported", got, "stopped")
	}
}

// TestAnActionThatOnlyFailedMovesNobody: an action with nothing to say names no outcome, and the
// subject stays where its conditions can still reach it.
func TestAnActionThatOnlyFailedMovesNobody(t *testing.T) {
	m, at := actingMachine(t, func(context.Context, int) (Outcome, error) {
		return Outcome{}, errors.New("no answer at all")
	})
	m.Look("subject")
	if got := at.get(); got != "acting" {
		t.Fatalf("state = %q, want %q", got, "acting")
	}
}

// silent drops the machine's account: what this test reads is where the subject ends up.
type silent struct{}

func (silent) Record(Entry) {}

// actingMachine is one acting state that leads to "stopped" on a failure, plus where the subject
// stands, read back.
func actingMachine(t *testing.T, do Doer[int], opts ...func(*Config[int])) (Machine[int], *storedState) {
	t.Helper()
	cfg := configFor(t, do, opts...)
	at := cfg.stored
	m, err := New(context.Background(), cfg.Config)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, at
}

// testConfig is a Config plus the row it reads and writes, so a test can seed a stale state name.
type testConfig struct {
	Config[int]
	stored *storedState
}

// configFor builds the flow under test without starting it, for a case about the startup checks.
func configFor(t *testing.T, do Doer[int], opts ...func(*Config[int])) testConfig {
	t.Helper()
	at := &storedState{name: "acting"}
	cfg := Config[int]{
		Start: "acting",
		States: []State[int]{
			{Name: "acting", Title: "Acting", About: "the state under test", Action: failing, Events: []Transition[int]{
				{On: done, To: "stopped", Why: "it worked"},
				{On: failed, To: "stopped", Why: "it did not, and that is an answer too"},
				{On: never, To: "stopped", Why: "the condition exit every acting state owes"},
			}},
			{Name: "stopped", Title: "Stopped", About: "where a failure lands"},
		},
		Gather:   func(string) (int, error) { return 0, nil },
		Stored:   func(string) (string, time.Time, error) { return at.get(), time.Now(), nil },
		Move:     func(_, _, to, _ string) error { at.set(to); return nil },
		Do:       map[string]Doer[int]{failing.Name: do},
		Subjects: func() []string { return []string{"subject"} },
		Default:  time.Minute,
		Record:   silent{},
	}
	for _, o := range opts {
		o(&cfg)
	}
	return testConfig{Config: cfg, stored: at}
}

// storedState stands in for the row a domain keeps. Guarded, because an action's own goroutine
// writes it through Move while the test reads it.
type storedState struct {
	mu   sync.Mutex
	name string
}

func (s *storedState) get() string  { s.mu.Lock(); defer s.mu.Unlock(); return s.name }
func (s *storedState) set(n string) { s.mu.Lock(); defer s.mu.Unlock(); s.name = n }

// TestASupersededStateMovesToItsReplacement: editing a flow strands every subject whose stored row
// names a state the new map does not have, and a row nothing can resolve fails every call that
// subject makes for as long as it stands. thrain sat in `worker/mail` after that state was removed,
// and `sindri` answered "cannot reach the hub" from a hub that was running perfectly well.
func TestASupersededStateMovesToItsReplacement(t *testing.T) {
	m, at := actingMachine(t, func(context.Context, int) (Outcome, error) { return done, nil },
		func(c *Config[int]) { c.Superseded = map[string]string{"gone": "stopped"} })
	at.set("gone") // a row written by a flow that still declared it

	s, err := m.State("subject")
	if err != nil {
		t.Fatalf("a superseded name must be migrated, not returned as an error: %v", err)
	}
	if s.Name != "stopped" {
		t.Errorf("state = %q, want its declared replacement %q", s.Name, "stopped")
	}
	if got := at.get(); got != "stopped" {
		t.Errorf("the migration must be WRITTEN, or the next call strands it again: %q", got)
	}
}

// TestAnUnlistedDeadStateFallsBackToTheStart: nobody said where this one went, so the start is the
// only safe answer — the subject is rescued and the missing entry is what the record then says.
func TestAnUnlistedDeadStateFallsBackToTheStart(t *testing.T) {
	m, at := actingMachine(t, func(context.Context, int) (Outcome, error) { return done, nil })
	at.set("never-heard-of-it")

	if s, err := m.State("subject"); err != nil || s.Name != "acting" {
		t.Fatalf("state = %+v (err %v), want the start state", s.Name, err)
	}
	if got := at.get(); got != "acting" {
		t.Errorf("the rescue must be written: %q", got)
	}
}

// TestASupersededEntryMustNameADeclaredState: an entry pointing at nothing is a migration that
// strands exactly the subjects it was written to rescue, so the flow refuses to start.
func TestASupersededEntryMustNameADeclaredState(t *testing.T) {
	_, err := New(context.Background(), configFor(t, func(context.Context, int) (Outcome, error) { return done, nil },
		func(c *Config[int]) { c.Superseded = map[string]string{"gone": "nowhere"} }).Config)
	if err == nil {
		t.Fatal("a superseded state pointing at an undeclared one must fail at startup")
	}
}

// TestALiveStateCannotBeSuperseded: listing one the map still declares means two answers for where
// a subject standing there belongs.
func TestALiveStateCannotBeSuperseded(t *testing.T) {
	_, err := New(context.Background(), configFor(t, func(context.Context, int) (Outcome, error) { return done, nil },
		func(c *Config[int]) { c.Superseded = map[string]string{"acting": "stopped"} }).Config)
	if err == nil {
		t.Fatal("a declared state listed as superseded must fail at startup")
	}
}
