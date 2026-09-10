package fleet

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestPrepareAssignmentDeliversAfterAModelSwitch: the switch blocks until it has happened, and only
// then is the claimed directive delivered — nothing rides along inside the command. fired must read
// true so the caller names the running action rather than dir, which the switch's own clear would wipe.
func TestPrepareAssignmentDeliversAfterAModelSwitch(t *testing.T) {
	deps := &stubDeps{tierModels: map[string]string{"mid": "big-model"}, currentModel: "small-model"}
	e := storelessEngine(t, deps)

	fired, err := e.roleAct().PrepareAssignment(t.Context(), "repo", "dvalin", "mid", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if !fired {
		t.Error("fired = false, want true — a model switch is due")
	}
	if len(deps.modelSet) != 1 || deps.modelSet[0] != "dvalin=big-model" {
		t.Errorf("modelSet = %v, want dvalin switched to big-model", deps.modelSet)
	}
	if len(deps.injectedText) != 1 || deps.injectedText[0] != "you hold td-abc123" {
		t.Errorf("injectedText = %v, want the claimed directive delivered once the switch answered", deps.injectedText)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — SetModel covers that half itself", deps.compacted)
	}
}

// TestPrepareAssignmentSendsNothingAfterAFailedSwitch is what having an answer buys: a switch that
// failed leaves the agent on the old model, so handing it the directive anyway would run the work on
// a session the caller believes was reset.
func TestPrepareAssignmentSendsNothingAfterAFailedSwitch(t *testing.T) {
	deps := &stubDeps{
		tierModels: map[string]string{"mid": "big-model"}, currentModel: "small-model",
		setModelErr: errors.New("boom"),
	}
	e := storelessEngine(t, deps)

	if _, err := e.roleAct().PrepareAssignment(t.Context(), "repo", "dvalin", "mid", "dir"); err == nil {
		t.Fatal("prepareAssignment: want the underlying SetModel error surfaced")
	}
	if len(deps.injectedText) != 0 {
		t.Errorf("injectedText = %v, want nothing delivered behind a switch that failed", deps.injectedText)
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
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, clearErr: errors.New("clear timed out")}
	e := newEngine(st, deps)

	fired, err := e.roleAct().PrepareAssignment(t.Context(), "repo", "dvalin", "mid", "you hold td-abc123")
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
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e := storelessEngine(t, deps)

	fired, err := e.roleAct().PrepareAssignment(t.Context(), "repo", "dvalin", "mid", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if !fired {
		t.Error("fired = false, want true — the session holds something to discard")
	}
	if len(deps.cleared) != 1 || deps.cleared[0] != "dvalin" {
		t.Errorf("cleared = %v, want exactly one Clear(dvalin) fired", deps.cleared)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — preparation is the clear, never compaction", deps.compacted)
	}
	if len(deps.injectedText) != 1 || deps.injectedText[0] != "you hold td-abc123" {
		t.Errorf("injectedText = %v, want the claimed directive delivered after the clear", deps.injectedText)
	}
}

// TestPrepareAssignmentClearsAFullSessionToo: a nearly-full session prepares the same way a lightly
// used one does — the clear — so nothing about fill changes which preparation fires.
func TestPrepareAssignmentClearsAFullSessionToo(t *testing.T) {
	deps := &stubDeps{ctxTokens: 900_000, ctxWindow: 1_000_000, ctxOK: true, compactThreshold: 75_000}
	e := storelessEngine(t, deps)

	fired, err := e.roleAct().PrepareAssignment(t.Context(), "repo", "dvalin", "mid", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if !fired {
		t.Error("fired = false, want true — the session holds something to discard")
	}
	if len(deps.cleared) != 1 || deps.cleared[0] != "dvalin" {
		t.Errorf("cleared = %v, want exactly one Clear(dvalin) fired", deps.cleared)
	}
	if len(deps.injectedText) != 1 || deps.injectedText[0] != "you hold td-abc123" {
		t.Errorf("injectedText = %v, want the claimed directive delivered once the clear answered", deps.injectedText)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — preparation is the clear, never compaction", deps.compacted)
	}
}

// TestPrepareAssignmentSkipsBothOnAnEmptySession: no tier mismatch and nothing to discard fires
// nothing and reports fired=false, so the caller answers with dir directly.
func TestPrepareAssignmentSkipsBothOnAnEmptySession(t *testing.T) {
	deps := &stubDeps{ctxTokens: 0, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e := storelessEngine(t, deps)

	fired, err := e.roleAct().PrepareAssignment(t.Context(), "repo", "dvalin", "mid", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if fired {
		t.Error("fired = true, want false — nothing is due")
	}
	if len(deps.cleared) != 0 || len(deps.modelSet) != 0 {
		t.Errorf("cleared = %v, modelSet = %v, want neither to have fired", deps.cleared, deps.modelSet)
	}
}

// TestPrepareAssignmentToleratesADatedCurrentModel: CurrentModel may report a more specific id than
// ModelForTier's plain one (a dated snapshot suffix) — ModelMatches tells the two apart, so a bare
// equality doesn't re-switch (clearing the session) on every claim already on the right model.
func TestPrepareAssignmentToleratesADatedCurrentModel(t *testing.T) {
	deps := &stubDeps{
		tierModels:   map[string]string{"junior": "claude-haiku-4-5"},
		currentModel: "claude-haiku-4-5-20251001",
	}
	e := storelessEngine(t, deps)

	fired, err := e.roleAct().PrepareAssignment(t.Context(), "repo", "dvalin", "junior", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if fired {
		t.Error("fired = true, want false — already running the tier's model, just under a more specific id")
	}
	if len(deps.modelSet) != 0 {
		t.Errorf("modelSet = %v, want none", deps.modelSet)
	}
}
