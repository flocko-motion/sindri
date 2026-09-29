package machine

import (
	"context"
	"testing"
	"time"
)

// weighed is a resting state with two conditions on a world of 7 — one false, one true — and an
// outcome, which is never read off the world.
func weighed(t *testing.T) (Machine[int], *storedState) {
	t.Helper()
	at := &storedState{name: "watching"}
	is7 := Condition[int]{Name: "is-seven", Within: time.Minute, Holds: func(w int) bool { return w == 7 },
		Wake: []Topic{"counted"}}
	is8 := Condition[int]{Name: "is-eight", Within: time.Minute, Holds: func(w int) bool { return w == 8 }}
	m, err := New(context.Background(), Config[int]{
		Start: "watching",
		States: []State[int]{
			{Name: "watching", Title: "Watching", About: "reads the world", WhenIdle: LetItRest, Events: []Transition[int]{
				{On: is8, To: "eight", Why: "it reads eight"},
				{On: is7, To: "seven", Why: "it reads seven"},
				{On: done, To: Stay, Why: "an outcome, which no world holds"},
			}},
			{Name: "seven", Title: "Seven", About: "seven"},
			{Name: "eight", Title: "Eight", About: "eight"},
		},
		Gather: func(string) (int, error) { return 7, nil },
		Stored: func(string) (string, time.Time, error) { return at.get(), time.Now(), nil },
		Move:   func(_, _, to, _ string) error { at.set(to); return nil },
		Do:     map[string]Doer[int]{}, Subjects: func() []string { return []string{"s"} },
		Default: time.Minute, Record: silent{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, at
}

// TestWeighReadsEveryExitAndMovesNobody: the debug view shows which exits hold without the reading
// being a pass — a view that moved an agent by looking at it would be the very hiccup it exists for.
func TestWeighReadsEveryExitAndMovesNobody(t *testing.T) {
	m, at := weighed(t)
	s, holds, err := m.Weigh("s")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "watching" {
		t.Errorf("state = %q, want watching", s.Name)
	}
	want := []bool{false, true, false}
	if len(holds) != len(want) {
		t.Fatalf("holds = %v, want %v (one per event, in declaration order)", holds, want)
	}
	for i := range want {
		if holds[i] != want[i] {
			t.Errorf("holds[%d] = %v, want %v", i, holds[i], want[i])
		}
	}
	if got := at.get(); got != "watching" {
		t.Errorf("weighing moved the subject to %q", got)
	}
}

// TestGraphExportsEveryEdgeWithItsKind: the drawn map must be the declared one, edge for edge and in
// order, since the first exit that holds is the one taken.
func TestGraphExportsEveryEdgeWithItsKind(t *testing.T) {
	m, _ := weighed(t)
	g, _ := Graph(m.States(), nil)
	if len(g) != 3 || g[0].Name != "watching" {
		t.Fatalf("graph = %+v, want the three states in declaration order", g)
	}
	evs := g[0].Events
	if len(evs) != 3 {
		t.Fatalf("events = %+v, want 3", evs)
	}
	if evs[0].On != "is-eight" || evs[0].Trigger != "condition" || evs[0].To != "eight" || evs[0].Within != "1m0s" {
		t.Errorf("first edge = %+v", evs[0])
	}
	if len(evs[1].Wake) != 1 || evs[1].Wake[0] != "counted" {
		t.Errorf("second edge's wake topics = %v, want [counted]", evs[1].Wake)
	}
	if evs[2].Trigger != "outcome" || evs[2].To != "" {
		t.Errorf("third edge = %+v, want an outcome that moves nobody", evs[2])
	}
	if g[0].WhenIdle != "rest" {
		t.Errorf("whenIdle = %q, want rest", g[0].WhenIdle)
	}
}
