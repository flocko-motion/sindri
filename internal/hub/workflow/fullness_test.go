package workflow

import (
	"errors"
	"reflect"
	"testing"
)

// TestPrepareAssignmentBracketsAModelSwitch: BeginAssignment must stay open for the whole SetModel
// call, admitting the claim just written past AtLeafBoundary's guard. dir queues behind the live
// switch, and fired must read true so the caller answers DirPreparing, not dir directly.
func TestPrepareAssignmentBracketsAModelSwitch(t *testing.T) {
	deps := &stubDeps{tierModels: map[string]string{"mid": "big-model"}, currentModel: "small-model"}
	e := New(nil, deps)

	fired, err := e.prepareAssignment("repo", "dvalin", "mid", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if !fired {
		t.Error("fired = false, want true — a model switch is due")
	}
	want := []string{"begin:dvalin", "end:dvalin"}
	if !reflect.DeepEqual(deps.assignBrackets, want) {
		t.Errorf("assignBrackets = %v, want %v", deps.assignBrackets, want)
	}
	if len(deps.modelSet) != 1 || deps.modelSet[0] != "dvalin=big-model" {
		t.Errorf("modelSet = %v, want dvalin switched to big-model", deps.modelSet)
	}
	if len(deps.modelSetWith) != 1 || deps.modelSetWith[0] != "you hold td-abc123" {
		t.Errorf("modelSetWith = %v, want the claimed directive passed as SetModel's next", deps.modelSetWith)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — SetModel covers that half itself", deps.compacted)
	}
}

// TestPrepareAssignmentClosesTheBracketEvenOnError: a SetModel failure must not leave the flag
// standing, or AtLeafBoundary admits a claim forever past the preparation it was opened for.
func TestPrepareAssignmentClosesTheBracketEvenOnError(t *testing.T) {
	deps := &stubDeps{
		tierModels: map[string]string{"mid": "big-model"}, currentModel: "small-model",
		setModelErr: errors.New("boom"),
	}
	e := New(nil, deps)

	if _, err := e.prepareAssignment("repo", "dvalin", "mid", "dir"); err == nil {
		t.Fatal("prepareAssignment: want the underlying SetModel error surfaced")
	}
	want := []string{"begin:dvalin", "end:dvalin"}
	if !reflect.DeepEqual(deps.assignBrackets, want) {
		t.Errorf("assignBrackets = %v, want %v — the bracket must close even on error", deps.assignBrackets, want)
	}
}

// TestPrepareAssignmentBracketsACompaction: no model switch wanted, so the bracket covers Compact's
// own boundary check instead. dir queues behind /compact, and fired must read true.
func TestPrepareAssignmentBracketsACompaction(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e := New(nil, deps)

	fired, err := e.prepareAssignment("repo", "dvalin", "mid", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if !fired {
		t.Error("fired = false, want true — compaction is due")
	}
	want := []string{"begin:dvalin", "end:dvalin"}
	if !reflect.DeepEqual(deps.assignBrackets, want) {
		t.Errorf("assignBrackets = %v, want %v", deps.assignBrackets, want)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != "dvalin" {
		t.Errorf("compacted = %v, want exactly one Compact(dvalin) fired", deps.compacted)
	}
	if len(deps.compactedWith) != 1 || deps.compactedWith[0] != "you hold td-abc123" {
		t.Errorf("compactedWith = %v, want the claimed directive queued behind /compact", deps.compactedWith)
	}
}

// TestPrepareAssignmentBracketsAClear: no model switch wanted, fill past ContextFullFraction, so
// the bracket covers FireClear instead of a compaction — the same admit-the-claim reason applies to
// FireClear's own boundary check, and a clear wins over a compaction whenever both would apply.
func TestPrepareAssignmentBracketsAClear(t *testing.T) {
	deps := &stubDeps{ctxTokens: 900_000, ctxWindow: 1_000_000, ctxOK: true, compactThreshold: 75_000}
	e := New(nil, deps)

	fired, err := e.prepareAssignment("repo", "dvalin", "mid", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if !fired {
		t.Error("fired = false, want true — past ContextFullFraction")
	}
	want := []string{"begin:dvalin", "end:dvalin"}
	if !reflect.DeepEqual(deps.assignBrackets, want) {
		t.Errorf("assignBrackets = %v, want %v", deps.assignBrackets, want)
	}
	if len(deps.cleared) != 1 || deps.cleared[0] != "dvalin" {
		t.Errorf("cleared = %v, want exactly one FireClear(dvalin) fired", deps.cleared)
	}
	if len(deps.clearedWith) != 1 || deps.clearedWith[0] != "you hold td-abc123" {
		t.Errorf("clearedWith = %v, want the claimed directive queued behind /clear, same as Compact's own next", deps.clearedWith)
	}
	if len(deps.clearedInterrupt) != 1 || deps.clearedInterrupt[0] {
		t.Errorf("clearedInterrupt = %v, want false — firing here runs inside the agent's own ask, "+
			"and ESC would cut off the very turn computing DirPreparing", deps.clearedInterrupt)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — the clear pre-empts it", deps.compacted)
	}
}

// TestPrepareAssignmentSkipsBothWhenNeitherApplies: the ordinary case — no tier mismatch, fill
// under threshold — fires nothing and reports fired=false, so the caller answers with dir directly.
func TestPrepareAssignmentSkipsBothWhenNeitherApplies(t *testing.T) {
	deps := &stubDeps{ctxTokens: 1_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e := New(nil, deps)

	fired, err := e.prepareAssignment("repo", "dvalin", "mid", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if fired {
		t.Error("fired = true, want false — nothing is due")
	}
	if len(deps.compacted) != 0 || len(deps.modelSet) != 0 {
		t.Errorf("compacted = %v, modelSet = %v, want neither to have fired", deps.compacted, deps.modelSet)
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
	e := New(nil, deps)

	fired, err := e.prepareAssignment("repo", "dvalin", "junior", "you hold td-abc123")
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
