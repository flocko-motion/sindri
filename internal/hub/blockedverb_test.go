package hub

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// callerFor resolves an agent's command-surface identity, for asserting on the surface itself
// rather than on the output of running something.
func callerFor(t *testing.T, h *Hub, agent string) registry.Caller {
	t.Helper()
	c, err := h.caller(testProject, agent)
	if err != nil {
		t.Fatalf("caller %s: %v", agent, err)
	}
	return c
}

// exec runs a verb as an agent and returns what the agent would see, plus the exit code.
func execAs(t *testing.T, h *Hub, agent string, args ...string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	code, err := h.AgentExec(testProject, agent, args, &out)
	if err != nil {
		t.Fatalf("AgentExec %v: %v", args, err)
	}
	return out.String(), code
}

// TestBlockedVerbExplainsItself is what a feature worker hit in the field: told to submit, it found
// submit absent from its surface and had to work out on its own that checkpoint was the verb it
// held. A verb the state machine holds back is still a verb the agent may have been pointed at, so
// asking for it must answer — why not now, and what to run instead.
func TestBlockedVerbExplainsItself(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	// Mid-feature: one subtask still open, so the branch is incomplete and submit waits for it.
	for _, task := range []store.Task{
		{ID: "td-EPIC", Title: "a feature", Status: "open", Priority: "P1", Type: "epic"},
		{ID: "td-1", Title: "a subtask", Status: "open", Priority: "P1", ParentID: "td-EPIC"},
	} {
		if err := ps.UpsertTask(task); err != nil {
			t.Fatalf("seed %s: %v", task.ID, err)
		}
	}
	if err := ps.SetState(store.AgentState{
		Agent: "dvalin", Container: "td-EPIC", Branch: "td-EPIC", Task: "td-1", Phase: "working",
	}); err != nil {
		t.Fatalf("set state: %v", err)
	}

	out, code := execAs(t, h, "dvalin", "submit", "done with it")
	if code == 0 {
		t.Error("submit must not run while the feature has subtasks left")
	}
	for _, want := range []string{"isn't available right now", "td-EPIC", "`sindri checkpoint"} {
		if !strings.Contains(out, want) {
			t.Errorf("the refusal should carry %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "unknown") {
		t.Errorf("a real verb held back by state must not read as unknown:\n%s", out)
	}

	// Close that subtask and submit opens up: a finished feature is put up by the worker that built
	// it, the same as any task. This is the whole of what a human used to have to do by hand.
	if err := ps.UpsertTask(store.Task{
		ID: "td-1", Title: "a subtask", Status: "closed", Priority: "P1", ParentID: "td-EPIC",
	}); err != nil {
		t.Fatalf("close subtask: %v", err)
	}
	if _, _, ok := h.registry().Resolve("submit", callerFor(t, h, "dvalin")); !ok {
		t.Error("submit must be open once every subtask is checkpointed")
	}

	// The mirror case: the same explanation runs the other way for a worker on a task of its own.
	if err := ps.SetState(store.AgentState{
		Agent: "dvalin", Branch: "td-9", Task: "td-9", Phase: "working",
	}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	out, code = execAs(t, h, "dvalin", "checkpoint")
	if code == 0 {
		t.Error("checkpoint must not run for a worker with no feature")
	}
	if !strings.Contains(out, "`sindri submit") {
		t.Errorf("the refusal should point at submit:\n%s", out)
	}

	// A verb of another ROLE stays opaque: a worker learns nothing of the reviewer's surface.
	out, code = execAs(t, h, "dvalin", "approve", "pr-1")
	if code != 127 || !strings.Contains(out, "unknown") {
		t.Errorf("a reviewer verb should read as unknown to a worker (code=%d):\n%s", code, out)
	}
}
