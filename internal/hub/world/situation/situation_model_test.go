package situation

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/world/observe"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// fixedObserver answers with one reading, so a case can set what the transcript says against what
// the roster recorded.
type fixedObserver struct{ o observe.Observation }

func (f fixedObserver) Observe(_, _ string) observe.Observation { return f.o }

// TestTheSituationJoinsTheRecordWithTheTranscript: what an agent RUNS is one reading, joined here
// and nowhere else. Two readings is how a worker came to be retiered every twelve seconds for four
// days — its roster row naming a model, its session answering as another, and each rule picking
// whichever half it happened to reach for.
func TestTheSituationJoinsTheRecordWithTheTranscript(t *testing.T) {
	for _, c := range []struct {
		name     string
		up       bool
		detected string
		recorded string
		want     string
		why      string
	}{
		{"a live session disagreeing with its record", true, "claude-opus-5", "claude-sonnet-5", "claude-opus-5",
			"the transcript is what it answers as; the record is only what was chosen for it"},
		{"a switch made by hand in the pane", true, "claude-opus-5", "", "claude-opus-5",
			"the transcript sees a human's own /model before the roster ever does"},
		{"a stopped agent", false, "claude-opus-5", "claude-sonnet-5", "claude-sonnet-5",
			"nothing is running to disagree, so the choice is what its next launch carries"},
		{"a session that has not answered yet", true, "", "claude-sonnet-5", "claude-sonnet-5",
			"a fresh transcript names no model, which is silence rather than a different one"},
	} {
		t.Run(c.name, func(t *testing.T) {
			st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { st.Close() })
			if err := st.RegisterProject("repo", t.TempDir()); err != nil {
				t.Fatal(err)
			}
			if err := st.For("repo").PutAgent(store.Agent{Name: "dvalin", Role: "worker", Model: c.recorded}); err != nil {
				t.Fatal(err)
			}
			obs := fixedObserver{observe.Observation{TakenAt: time.Now(), Up: c.up, Model: c.detected}}

			sit, err := NewGatherer(st, obs, nil).Of("repo", "dvalin")
			if err != nil {
				t.Fatal(err)
			}

			if sit.Model != c.want {
				t.Errorf("Model = %q, want %q — %s", sit.Model, c.want, c.why)
			}
			// The evidence stays reachable underneath: a caller wanting what the transcript ACTUALLY
			// said still has it, which is what makes the joined reading safe to shadow it with.
			if sit.Observation.Model != c.detected {
				t.Errorf("Observation.Model = %q, want the raw reading %q", sit.Observation.Model, c.detected)
			}
		})
	}
}
