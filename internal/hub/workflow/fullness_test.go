package workflow

import (
	"errors"
	"reflect"
	"testing"
)

// TestPrepareAssignmentBracketsAModelSwitch: BeginAssignment must be open for the WHOLE SetModel
// call, since that is what admits the claim just written past AtLeafBoundary's own guard
// (-> agent.Service.AtLeafBoundary). dir is passed through as SetModel's own next, queued behind
// the live switch — no relaunch — and fired must read true so the caller answers with
// DirPreparing, not dir directly (the switch clears first, which would cut it off).
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
// standing — it would strand AtLeafBoundary admitting a claim forever, past the preparation it was
// opened for.
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

// TestPrepareAssignmentBracketsACompaction: no model switch wanted, so the bracket covers the
// compaction check instead — the same admit-the-claim reason applies to Compact's own boundary
// check. dir queues behind /compact as the real instruction, and fired must read true so the
// caller answers with DirPreparing.
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

// TestPrepareAssignmentToleratesADatedCurrentModel: CurrentModel may report a more specific id
// than ModelForTier's plain one (a dated snapshot suffix, in practice) — a bare equality would
// read that as a permanent mismatch and re-switch (clearing the session) on every claim, even
// already running on the right model. ModelMatches is what tells the two apart.
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
