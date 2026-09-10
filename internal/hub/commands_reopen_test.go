package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestPlannerCanReopenAClosedTask: the base case — a planner reopens a closed task it owns, with a
// reason, and that reason lands as a comment on the task's thread.
func TestPlannerCanReopenAClosedTask(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "galar", Role: "planner"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "sd-1", Title: "a task", Status: "closed", Priority: "P2"}); err != nil {
		t.Fatal(err)
	}
	out, code := execAs(t, h, "galar", "reopen-task", "sd-1", "the ARCHITECTURE.md edit never landed")
	if code != 0 {
		t.Fatalf("reopen-task failed (%d): %s", code, out)
	}
	got, ok, err := ps.OwnedTask("sd-1")
	if err != nil || !ok || got.Status != "open" {
		t.Fatalf("sd-1 = %+v, ok=%v err=%v, want status open", got, ok, err)
	}
	cs, err := ps.Comments("sd-1")
	if err != nil || len(cs) != 1 {
		t.Fatalf("comments = %v, err %v, want one", cs, err)
	}
	if !strings.Contains(cs[0].Body, "ARCHITECTURE.md edit never landed") {
		t.Errorf("the reason should be recorded on the task, got %q", cs[0].Body)
	}
	if cs[0].Author != "galar" {
		t.Errorf("author = %q, want galar", cs[0].Author)
	}
	// A standing priority makes it immediately claimable — the reopener must be told.
	if !strings.Contains(out, "priority") {
		t.Errorf("reply should mention the standing priority, got %q", out)
	}
}

// TestReopenTaskRefusesAnEmptyReason: the reason is the whole point of the capability, so an empty
// one must refuse rather than reopen silently.
func TestReopenTaskRefusesAnEmptyReason(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "galar", Role: "planner"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "sd-1", Title: "a task", Status: "closed", Priority: "P2"}); err != nil {
		t.Fatal(err)
	}
	if _, code := execAs(t, h, "galar", "reopen-task", "sd-1"); code == 0 {
		t.Error("reopen-task with no reason should be refused")
	}
	got, _, _ := ps.OwnedTask("sd-1")
	if got.Status != "closed" {
		t.Errorf("status = %q, want unchanged (closed) after a refused reopen", got.Status)
	}
}

// TestReopenTaskRefusesAGitHubID: scope is sindri-owned ids only — a gh- id's status comes from the
// issue tracker, not sindri's store.
func TestReopenTaskRefusesAGitHubID(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "galar", Role: "planner"}); err != nil {
		t.Fatal(err)
	}
	if _, code := execAs(t, h, "galar", "reopen-task", "gh-42", "wrong call"); code == 0 {
		t.Error("reopening a gh- id should be refused")
	}
}

// TestWorkerCannotReopenATask: never a worker — it could undo a human's verdict on its own task.
func TestWorkerCannotReopenATask(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	c := callerFor(t, h, "dvalin")
	if _, _, ok := h.registry().Resolve("reopen-task", c); ok {
		t.Error("a worker should not be able to resolve reopen-task")
	}
	for _, cmd := range h.registry().Available(c) {
		if cmd.Name == "reopen-task" {
			t.Error("a worker's surface should not list reopen-task")
		}
	}
}
