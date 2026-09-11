package task

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/world/situation"
)

// TestAnArmedReviewerIsNotHandedTheNextPR closes the door the sweep's gate left open: the rule the
// reviewer's own map reads before claiming is the surface's one, so an arming and a retirement are
// weighed in the place they are weighed for everything else (-> cond.ReviewWaiting).
func TestAnArmedReviewerIsNotHandedTheNextPR(t *testing.T) {
	armed := situation.Situation{Name: "fili", Role: "reviewer", ClearArmed: true}
	if situation.Allows(armed.Allowed().Assign) {
		t.Error("an armed reviewer must not be handed one — the clear is waiting for it to be free")
	}
	free := armed
	free.ClearArmed = false
	if !situation.Allows(free.Allowed().Assign) {
		t.Error("disarmed, the same reviewer takes reviews again")
	}
	// Retirement disqualifies one too, which is what folding this into the surface bought: the two
	// used to be checked in different functions, and one of them forgot.
	retired := situation.Situation{Name: "fili", Role: "reviewer", Retired: true}
	if situation.Allows(retired.Allowed().Assign) {
		t.Error("a retired reviewer must not be handed one either")
	}
}
