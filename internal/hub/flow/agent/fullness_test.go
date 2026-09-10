package agent

import (
	"errors"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// TestPrepareAssignmentDeliversAfterAModelSwitch: the switch blocks until it has happened, and only
// then is the claimed directive delivered — nothing rides along inside the command. fired must read
// true so the caller names the running action rather than dir, which the switch's own clear would wipe.
func TestPrepareAssignmentDeliversAfterAModelSwitch(t *testing.T) {
	deps := &flowtest.Hub{TierModels: map[string]string{"mid": "big-model"}, Model: "small-model"}
	a, _, _ := newActWith(t, deps)

	fired, err := a.PrepareAssignment(t.Context(), "repo", "dvalin", "mid", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if !fired {
		t.Error("fired = false, want true — a model switch is due")
	}
	if len(deps.ModelSet) != 1 || deps.ModelSet[0] != "dvalin=big-model" {
		t.Errorf("modelSet = %v, want dvalin switched to big-model", deps.ModelSet)
	}
	if len(deps.InjectedText) != 1 || deps.InjectedText[0] != "you hold td-abc123" {
		t.Errorf("injectedText = %v, want the claimed directive delivered once the switch answered", deps.InjectedText)
	}
}

// TestPrepareAssignmentSendsNothingAfterAFailedSwitch is what having an answer buys: a switch that
// failed leaves the agent on the old model, so handing it the directive anyway would run the work on
// a session the caller believes was reset.
func TestPrepareAssignmentSendsNothingAfterAFailedSwitch(t *testing.T) {
	deps := &flowtest.Hub{
		TierModels: map[string]string{"mid": "big-model"}, Model: "small-model",
		SetModelErr: errors.New("boom"),
	}
	a, _, _ := newActWith(t, deps)

	if _, err := a.PrepareAssignment(t.Context(), "repo", "dvalin", "mid", "dir"); err == nil {
		t.Fatal("prepareAssignment: want the underlying SetModel error surfaced")
	}
	if len(deps.InjectedText) != 0 {
		t.Errorf("injectedText = %v, want nothing delivered behind a switch that failed", deps.InjectedText)
	}
}

// TestAFailedClearStillHandsOverTheWork: /clear is typed into a session that is mid-turn, and
// awaitCleared gives up after clearSettleCap — 4 of jari's 5 model switches died there. A clear that
// never lands must cost the freshness, never the work, so this reports fired=false and leaves the
// caller to answer with the directive itself.
func TestAFailedClearStillHandsOverTheWork(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	deps := &flowtest.Hub{CtxTokens: 80_000, CtxWindow: 200_000, CtxOK: true, ClearErr: errors.New("clear timed out")}
	a, _, _ := newActWith(t, deps)

	fired, err := a.PrepareAssignment(t.Context(), "repo", "dvalin", "mid", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment surfaced the clear failure as its own: %v", err)
	}
	if fired {
		t.Error("fired = true, want false — nothing was prepared, so the caller answers with the directive")
	}
}

// TestPrepareAssignmentDeliversAfterTheClear: no model switch wanted, so the clear runs instead, and
// dir follows it. fired must read true.
func TestPrepareAssignmentDeliversAfterTheClear(t *testing.T) {
	deps := &flowtest.Hub{CtxTokens: 80_000, CtxWindow: 200_000, CtxOK: true}
	a, _, _ := newActWith(t, deps)

	fired, err := a.PrepareAssignment(t.Context(), "repo", "dvalin", "mid", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if !fired {
		t.Error("fired = false, want true — the session holds something to discard")
	}
	if len(deps.Cleared) != 1 || deps.Cleared[0] != "dvalin" {
		t.Errorf("cleared = %v, want exactly one Clear(dvalin) fired", deps.Cleared)
	}
	if len(deps.InjectedText) != 1 || deps.InjectedText[0] != "you hold td-abc123" {
		t.Errorf("injectedText = %v, want the claimed directive delivered after the clear", deps.InjectedText)
	}
}

// TestPrepareAssignmentClearsAFullSessionToo: a nearly-full session prepares the same way a lightly
// used one does — the clear — so nothing about fill changes which preparation fires.
func TestPrepareAssignmentClearsAFullSessionToo(t *testing.T) {
	deps := &flowtest.Hub{CtxTokens: 900_000, CtxWindow: 1_000_000, CtxOK: true}
	a, _, _ := newActWith(t, deps)

	fired, err := a.PrepareAssignment(t.Context(), "repo", "dvalin", "mid", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if !fired {
		t.Error("fired = false, want true — the session holds something to discard")
	}
	if len(deps.Cleared) != 1 || deps.Cleared[0] != "dvalin" {
		t.Errorf("cleared = %v, want exactly one Clear(dvalin) fired", deps.Cleared)
	}
	if len(deps.InjectedText) != 1 || deps.InjectedText[0] != "you hold td-abc123" {
		t.Errorf("injectedText = %v, want the claimed directive delivered once the clear answered", deps.InjectedText)
	}
}

// TestPrepareAssignmentSkipsBothOnAnEmptySession: no tier mismatch and nothing to discard fires
// nothing and reports fired=false, so the caller answers with dir directly.
func TestPrepareAssignmentSkipsBothOnAnEmptySession(t *testing.T) {
	deps := &flowtest.Hub{CtxTokens: 0, CtxWindow: 200_000, CtxOK: true}
	a, _, _ := newActWith(t, deps)

	fired, err := a.PrepareAssignment(t.Context(), "repo", "dvalin", "mid", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if fired {
		t.Error("fired = true, want false — nothing is due")
	}
	if len(deps.Cleared) != 0 || len(deps.ModelSet) != 0 {
		t.Errorf("cleared = %v, modelSet = %v, want neither to have fired", deps.Cleared, deps.ModelSet)
	}
}

// TestPrepareAssignmentToleratesADatedCurrentModel: CurrentModel may report a more specific id than
// ModelForTier's plain one (a dated snapshot suffix) — ModelMatches tells the two apart, so a bare
// equality doesn't re-switch (clearing the session) on every claim already on the right model.
func TestPrepareAssignmentToleratesADatedCurrentModel(t *testing.T) {
	deps := &flowtest.Hub{
		TierModels: map[string]string{"junior": "claude-haiku-4-5"},
		Model:      "claude-haiku-4-5-20251001",
	}
	a, _, _ := newActWith(t, deps)

	fired, err := a.PrepareAssignment(t.Context(), "repo", "dvalin", "junior", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if fired {
		t.Error("fired = true, want false — already running the tier's model, just under a more specific id")
	}
	if len(deps.ModelSet) != 0 {
		t.Errorf("modelSet = %v, want none", deps.ModelSet)
	}
}
