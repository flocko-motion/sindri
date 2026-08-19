package workflow

import (
	"errors"
	"reflect"
	"testing"
)

// TestPrepareAssignmentBracketsAModelSwitch: BeginAssignment must be open for the WHOLE SetModel
// call, since that is what admits the claim just written past AtLeafBoundary's own guard
// (-> agent.Service.AtLeafBoundary, agent.Service.SetModel's internal FireClear). dir is armed as
// the relaunch's own kickoff, since SetModel's restart kills this call's reply regardless of what
// it says, and fired must read true so the caller answers with DirPreparing, not dir directly.
func TestPrepareAssignmentBracketsAModelSwitch(t *testing.T) {
	deps := &stubDeps{tierModels: map[string]string{"mid": "big-model"}, currentModel: "small-model"}
	e := New(nil, deps)

	fired, err := e.prepareAssignment("repo", "dvalin", "mid", "you hold td-abc123")
	if err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	if !fired {
		t.Error("fired = false, want true — a model switch is about to relaunch the pod")
	}
	want := []string{"begin:dvalin", "end:dvalin"}
	if !reflect.DeepEqual(deps.assignBrackets, want) {
		t.Errorf("assignBrackets = %v, want %v", deps.assignBrackets, want)
	}
	if len(deps.modelSet) != 1 || deps.modelSet[0] != "dvalin=big-model" {
		t.Errorf("modelSet = %v, want dvalin switched to big-model", deps.modelSet)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — SetModel covers that half itself", deps.compacted)
	}
	if dir, ok := e.TakePendingKickoff("repo", "dvalin"); !ok || dir != "you hold td-abc123" {
		t.Errorf("TakePendingKickoff = (%q, %v), want the claimed directive armed for the relaunch", dir, ok)
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
// check. dir queues behind /compact as the real instruction, not armed as a kickoff (nothing
// relaunches here), and fired must read true so the caller answers with DirPreparing.
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
	if _, ok := e.TakePendingKickoff("repo", "dvalin"); ok {
		t.Error("TakePendingKickoff = ok, want nothing armed — compaction queues dir itself, no relaunch to wake into")
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
