package workflow

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestTheTierSurvivesTheWire is the defect: api.TaskReq carried no Tier field, so `--tier senior`
// was read by the CLI and dropped before the hub saw it. TierOrDefault then answered "mid", which
// reads as somebody's decision rather than a lost value — sd-176312 was filed senior and stored mid.
func TestTheTierSurvivesTheWire(t *testing.T) {
	spec := api.TaskSpec{Title: "a task", Tier: "senior", Priority: "P1"}
	req := api.TaskReq{Title: spec.Title, Type: spec.Type, Priority: spec.Priority, Tier: spec.Tier,
		Parent: spec.Parent, Description: spec.Description, Labels: spec.Labels}

	if got := req.Spec().Tier; got != "senior" {
		t.Errorf("Tier = %q after the round trip, want senior", got)
	}
}

// TestAnUnknownTierIsRefused: TierOrDefault answers "mid" for any word it does not know, so a
// mistyped or dropped tier arrived looking deliberate. Refusing is what makes the loss visible.
func TestAnUnknownTierIsRefused(t *testing.T) {
	e, _, _ := ownedEngine(t, "open")

	_, err := e.CreateTask("proj", api.TaskSpec{Title: "x", Tier: "expert"})
	if err == nil {
		t.Fatal("an unknown tier was accepted and would have silently become mid")
	}
	if !strings.Contains(err.Error(), "expert") || !strings.Contains(err.Error(), "senior") {
		t.Errorf("the refusal must name the word and the real ones, got %q", err)
	}

	// Every real tier passes, and unset stays a real answer — the task takes the default.
	for _, tier := range append(append([]string{}, api.TierWords...), "") {
		if _, err := e.CreateTask("proj", api.TaskSpec{Title: "x", Tier: tier}); err != nil {
			t.Errorf("tier %q was refused: %v", tier, err)
		}
	}
}
