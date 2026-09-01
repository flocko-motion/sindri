package situation

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// countingObserver answers with nothing and counts how often it was asked, so a test can tell one
// gather per agent from one per roster.
type countingObserver struct{ reads int }

func (o *countingObserver) Reading(_, _ string) Reading {
	o.reads++
	return Reading{Observed: true, Up: true}
}

// gatherFixture is one project with three agents and one claimable task.
func gatherFixture(t *testing.T) (*Gatherer, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("repo", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	for _, name := range []string{"dvalin", "eitri", "nori"} {
		if err := ps.PutAgent(store.Agent{Name: name, Role: "worker"}); err != nil {
			t.Fatal(err)
		}
	}
	// UpsertTask, not PutOwnedTask: the pool reads the cached task model, and the sync that fills it
	// from a source is the workflow's own step — the situation reports whatever the model holds.
	if err := ps.UpsertTask(store.Task{ID: "sd-1", Title: "waiting work", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	return NewGatherer(st, &countingObserver{}), ps
}

// TestARosterIsJudgedOnOneReadOfTheBacklog is the containment paying for itself: the claimable pool
// describes the PROJECT, so gathering a whole roster reads it once and every situation refers to that
// one reading. Re-read per agent it would turn one query into one per agent on every sweep, and the
// sweeps run over the whole fleet on a tick.
func TestARosterIsJudgedOnOneReadOfTheBacklog(t *testing.T) {
	g, _ := gatherFixture(t)
	roster, err := g.Roster("repo")
	if err != nil {
		t.Fatalf("Roster: %v", err)
	}
	if len(roster) != 3 {
		t.Fatalf("gathered %d situations, want one per agent", len(roster))
	}
	// The same slice header, not an equal copy: identity is what proves one read was shared rather
	// than three reads happening to agree.
	first := roster[0].Pool
	for _, s := range roster[1:] {
		if len(s.Pool.Leaves) != len(first.Leaves) || (len(first.Leaves) > 0 && &s.Pool.Leaves[0] != &first.Leaves[0]) {
			t.Errorf("%s was judged against a pool of its own — the roster must share one reading", s.Name)
		}
	}
	if len(first.Leaves) != 1 || first.Leaves[0].ID != "sd-1" {
		t.Errorf("pool leaves = %v, want the one claimable task", first.Leaves)
	}
}

// TestOneAgentStillGathersForItself: the single-agent form must work without a caller assembling
// anything, which is the other half of "it gathers for itself".
func TestOneAgentStillGathersForItself(t *testing.T) {
	g, _ := gatherFixture(t)
	s, err := g.Of("repo", "dvalin")
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	if s.Name != "dvalin" || s.Role != "worker" {
		t.Errorf("situation = %+v, want dvalin's own roster row", s)
	}
	if !s.Up {
		t.Error("the observer's reading must arrive with it, not be fetched by the caller")
	}
	if len(s.Pool.Leaves) != 1 {
		t.Errorf("pool leaves = %v, want the claimable task — the situation reads it itself", s.Pool.Leaves)
	}
}

// TestAnAgentNobodyHasHeardOfIsNotAnError: a name absent from the roster is an identity answer, and
// every rule over it reads as "holds nothing" rather than blowing up a sweep mid-roster.
func TestAnAgentNobodyHasHeardOfIsNotAnError(t *testing.T) {
	g, _ := gatherFixture(t)
	s, err := g.Of("repo", "ghost")
	if err != nil {
		t.Fatalf("Of on an unknown name: %v", err)
	}
	if s.Allowed().Assign != "" {
		t.Errorf("Assign = %q, want nothing refused for an agent that does not exist", s.Allowed().Assign)
	}
}
