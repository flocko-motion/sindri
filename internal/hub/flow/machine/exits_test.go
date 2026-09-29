package machine

import (
	"context"
	"strings"
	"testing"
	"time"
)

func holdsIf(name string, v int) Condition[int] {
	return Condition[int]{Name: name, Within: time.Minute, Holds: func(w int) bool { return w == v }}
}

// nested is a leaf inside an inner region inside an outer one, each with a First and a Then exit, on a
// world the test sets.
func nested(t *testing.T, world *int) (Machine[int], *storedState) {
	t.Helper()
	at := &storedState{name: "leaf"}
	to := func(c Condition[int]) Transition[int] {
		return Transition[int]{On: c, To: "out-" + c.Name, Why: c.Name}
	}
	outer, inner := "outer", "inner"
	states := []State[int]{{Name: "leaf", In: inner, Title: "Leaf", About: "the state under test",
		Events: []Transition[int]{to(holdsIf("own", 3))}}}
	for _, n := range []string{"outer-first", "inner-first", "own", "inner-then", "outer-then"} {
		states = append(states, State[int]{Name: "out-" + n, Title: n, About: "where " + n + " leads"})
	}
	m, err := New(context.Background(), Config[int]{
		Start: "leaf", States: states,
		Groups: []Group[int]{
			{Name: outer, Title: "Outer", About: "the outer region",
				First: []Transition[int]{to(holdsIf("outer-first", 1))}, Then: []Transition[int]{to(holdsIf("outer-then", 5))}},
			{Name: inner, In: outer, Title: "Inner", About: "the inner region",
				First: []Transition[int]{to(holdsIf("inner-first", 2))}, Then: []Transition[int]{to(holdsIf("inner-then", 4))}},
		},
		Gather: func(string) (int, error) { return *world, nil },
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

// TestInheritedExitsRunInWrappingOrder: a region's first word comes before everything inside it and
// its last after, so the order is outer First, inner First, the state's own, inner Then, outer Then.
func TestInheritedExitsRunInWrappingOrder(t *testing.T) {
	// World v makes exactly the exit numbered v hold, so the one true reading's position is its place.
	for v := 1; v <= 5; v++ {
		world := v
		m, _ := nested(t, &world)
		_, holds, err := m.Weigh("s")
		if err != nil {
			t.Fatal(err)
		}
		if len(holds) != 5 {
			t.Fatalf("leaf weighs %d exits, want 5 — its own and both regions' two", len(holds))
		}
		for i, h := range holds {
			if h != (i == v-1) {
				t.Errorf("world %d: exit %d holds=%v; the order is outer First, inner First, own, inner Then, outer Then", v, i+1, h)
			}
		}
	}
}

// TestAnInheritedFirstExitBeatsTheStatesOwn is the precedence the slots exist for: a human stepping in
// comes before anything the state itself would do next, and the region's closing word after it.
func TestAnInheritedFirstExitBeatsTheStatesOwn(t *testing.T) {
	for world, lands := range map[int]string{1: "out-outer-first", 2: "out-inner-first", 3: "out-own", 4: "out-inner-then", 5: "out-outer-then"} {
		w := world
		m, at := nested(t, &w)
		m.Look("s")
		if got := at.get(); got != lands {
			t.Errorf("world %d: moved to %q, want %q", world, got, lands)
		}
	}
}

// TestExitsListsTheEffectiveOrderWithOwners: the view draws each exit from whoever declared it, so the
// resolution names the owner and its place in that owner's list.
func TestExitsListsTheEffectiveOrderWithOwners(t *testing.T) {
	groups := map[string]Group[int]{
		"outer": {Name: "outer", First: []Transition[int]{{On: holdsIf("a", 1)}}, Then: []Transition[int]{{On: holdsIf("e", 5)}}},
		"inner": {Name: "inner", In: "outer", First: []Transition[int]{{On: holdsIf("b", 2)}}, Then: []Transition[int]{{On: holdsIf("d", 4)}}},
	}
	s := State[int]{Name: "leaf", In: "inner", Events: []Transition[int]{{On: holdsIf("c", 3)}}}
	var got []string
	for _, e := range Exits(groups, s) {
		got = append(got, e.Owner+":"+e.On.EventName())
	}
	want := "outer:a inner:b leaf:c inner:d outer:e"
	if strings.Join(got, " ") != want {
		t.Errorf("Exits = %v, want %s", got, want)
	}
}

// TestAGroupRefusesWhatBelongsToOneAction: an outcome or an orphan leads somewhere different from
// each state's own action, so a region declaring one is refused before the flow runs.
func TestAGroupRefusesWhatBelongsToOneAction(t *testing.T) {
	cases := map[string]Group[int]{
		"outcome":    {Name: "g", Title: "G", About: "a region", First: []Transition[int]{{On: done, To: "stopped", Why: "x"}}},
		"orphan":     {Name: "g", Title: "G", About: "a region", Then: []Transition[int]{{On: Orphaned{}, To: "stopped", Why: "x"}}},
		"no parent":  {Name: "g", In: "nowhere", Title: "G", About: "a region"},
		"no target":  {Name: "g", Title: "G", About: "a region", First: []Transition[int]{{On: never, To: "nowhere", Why: "x"}}},
		"no content": {Name: "g"},
	}
	for name, g := range cases {
		cfg := configFor(t, nil, func(c *Config[int]) { c.Groups = []Group[int]{g} })
		if _, err := New(context.Background(), cfg.Config); err == nil {
			t.Errorf("%s: a group declaring it was accepted", name)
		}
	}
}

// TestARegionCannotSitInsideItself: a cycle would make inheritance loop for ever.
func TestARegionCannotSitInsideItself(t *testing.T) {
	cfg := configFor(t, nil, func(c *Config[int]) {
		c.Groups = []Group[int]{
			{Name: "a", In: "b", Title: "A", About: "one region"},
			{Name: "b", In: "a", Title: "B", About: "another region"},
		}
	})
	if _, err := New(context.Background(), cfg.Config); err == nil || !strings.Contains(err.Error(), "inside itself") {
		t.Errorf("a cycle of regions: err = %v, want it refused", err)
	}
}
