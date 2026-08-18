package workflow

import (
	"errors"
	"reflect"
	"testing"
)

// TestPrepareAssignmentBracketsAModelSwitch: BeginAssignment must be open for the WHOLE SetModel
// call, since that is what admits the claim just written past AtLeafBoundary's own guard
// (-> agent.Service.AtLeafBoundary, agent.Service.SetModel's internal Compact).
func TestPrepareAssignmentBracketsAModelSwitch(t *testing.T) {
	deps := &stubDeps{tierModels: map[string]string{"mid": "big-model"}, currentModel: "small-model"}
	e := New(nil, deps)

	if err := e.prepareAssignment("repo", "dvalin", "mid"); err != nil {
		t.Fatalf("prepareAssignment: %v", err)
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

	if err := e.prepareAssignment("repo", "dvalin", "mid"); err == nil {
		t.Fatal("prepareAssignment: want the underlying SetModel error surfaced")
	}
	want := []string{"begin:dvalin", "end:dvalin"}
	if !reflect.DeepEqual(deps.assignBrackets, want) {
		t.Errorf("assignBrackets = %v, want %v — the bracket must close even on error", deps.assignBrackets, want)
	}
}

// TestPrepareAssignmentBracketsACompaction: no model switch wanted, so the bracket covers the
// compaction check instead — the same admit-the-claim reason applies to Compact's own boundary check.
func TestPrepareAssignmentBracketsACompaction(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e := New(nil, deps)

	if err := e.prepareAssignment("repo", "dvalin", "mid"); err != nil {
		t.Fatalf("prepareAssignment: %v", err)
	}
	want := []string{"begin:dvalin", "end:dvalin"}
	if !reflect.DeepEqual(deps.assignBrackets, want) {
		t.Errorf("assignBrackets = %v, want %v", deps.assignBrackets, want)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != "dvalin" {
		t.Errorf("compacted = %v, want exactly one Compact(dvalin) fired", deps.compacted)
	}
}
