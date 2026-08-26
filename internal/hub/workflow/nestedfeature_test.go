package workflow

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// nestedFeature reproduces the tree that broke: a feature whose direct child is itself an epic with
// children of its own, which is how a planner writes up anything larger than a couple of steps.
//
//	os-feat
//	├── td-flat   (a leaf)
//	└── td-mid    (an epic)
//	    ├── td-deep1
//	    └── td-deep2
func nestedFeature(t *testing.T) (*Engine, *store.ProjectStore) {
	t.Helper()
	const agent = "dain"
	root, _ := newWorkRepo(t, agent, "os-feat")
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: filepath.Join(".worktrees", agent)}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	for _, x := range []struct{ id, parent, typ string }{
		{"os-feat", "", "spec"}, {"td-flat", "os-feat", "task"}, {"td-mid", "os-feat", "epic"},
		{"td-deep1", "td-mid", "task"}, {"td-deep2", "td-mid", "task"},
	} {
		prio := ""
		if x.id == "os-feat" {
			prio = "P1" // the feature's rating releases its whole tree; children stay unrated
		}
		if err := ps.UpsertTask(store.Task{
			ID: x.id, Title: x.id, Status: "open", ParentID: x.parent, Type: x.typ, Priority: prio,
		}); err != nil {
			t.Fatalf("seed %s: %v", x.id, err)
		}
		if strings.HasPrefix(x.id, "td-") { // owned rows, so a checkpoint can close them
			if err := ps.PutOwnedTask(store.OwnedTask{ID: x.id, Title: x.id, Status: "open"}); err != nil {
				t.Fatalf("own %s: %v", x.id, err)
			}
			if err := ps.SetParent(x.id, x.parent); err != nil {
				t.Fatalf("parent %s: %v", x.id, err)
			}
		}
	}
	return New(st, &stubDeps{root: root}), ps
}

// TestAnEpicIsNeverHandedOutAsASubtask is the defect behind "how can the parent be done but not the
// children?". A feature served its DIRECT children, so a nested epic was handed out as though it
// were a piece of work, and the checkpoint that finished it wrote closed over four open children of
// its own. The grandchildren were meanwhile unreachable by any route: too deep for the feature loop,
// and barred from the leaf pool by an open parent.
func TestAnEpicIsNeverHandedOutAsASubtask(t *testing.T) {
	_, ps := nestedFeature(t)
	open, err := ps.OpenSubtasks("os-feat")
	if err != nil {
		t.Fatalf("OpenSubtasks: %v", err)
	}
	got := map[string]bool{}
	for _, x := range open {
		got[x.ID] = true
	}
	for _, want := range []string{"td-flat", "td-deep1", "td-deep2"} {
		if !got[want] {
			t.Errorf("%s is work inside the feature and must be offered; got %v", want, got)
		}
	}
	if got["td-mid"] {
		t.Error("td-mid is a parent with open children — it is not work, and closing it would lie")
	}
}

// TestCheckpointCarriesOnPastAParentAndClosesOneItCompletes covers both halves of the invariant: a
// task with open work under it cannot be finished, and a parent IS finished the moment its last
// child is — otherwise it stays open forever and, having no open children, later reads as a leaf.
//
// The first half used to be a REFUSAL, which had nowhere to go: the agent cannot close the child,
// re-parent it or approve anything, so its only exit was a human noticing. The work is inside the
// same feature on the same branch, so the checkpoint records it, leaves the parent open, and hands
// over the next leaf — the parent still never closes over its children, which is the invariant.
func TestCheckpointCarriesOnPastAParentAndClosesOneItCompletes(t *testing.T) {
	e, ps := nestedFeature(t)
	c := registry.Caller{Project: "repo", Agent: "dain", Role: "worker", Phase: "working"}
	hold := func(task string) {
		t.Helper()
		if err := ps.SetState(store.AgentState{
			Agent: "dain", Container: "os-feat", Branch: "os-feat", Task: task, Phase: "working",
		}, store.ReasonClaimed, "test setup"); err != nil {
			t.Fatalf("hold %s: %v", task, err)
		}
	}

	// td-mid has two open children, so no checkpoint can call it done — but the agent is carried on
	// to the work rather than stopped in front of it.
	hold("td-mid")
	var out strings.Builder
	if code, err := e.CmdCheckpoint(c, []string{"done"}, &out); code != 0 || err != nil {
		t.Fatalf("checkpointing a parent should carry on, not refuse: code=%d err=%v out=%s", code, err, out.String())
	}
	for _, want := range []string{"td-mid", "stays OPEN", "td-deep1"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the reply should say why %q is not finished and what is next: %s", want, out.String())
		}
	}
	if s, _, _ := ps.OwnedTask("td-mid"); s.Status == "closed" {
		t.Fatal("td-mid must not be closed over its open children")
	}
	if s, _ := ps.GetState("dain"); s.Task != "td-deep1" {
		t.Errorf("the agent should be on the work it gained, got %q", s.Task)
	}

	// Its children, one at a time. The first leaves td-mid open; the second completes it.
	hold("td-deep1")
	if code, err := e.CmdCheckpoint(c, []string{"first"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("checkpoint td-deep1: code=%d err=%v", code, err)
	}
	if s, _, _ := ps.OwnedTask("td-mid"); s.Status != "open" {
		t.Errorf("td-mid status = %q after one of two children, want open", s.Status)
	}
	hold("td-deep2")
	if code, err := e.CmdCheckpoint(c, []string{"second"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("checkpoint td-deep2: code=%d err=%v", code, err)
	}
	if s, _, _ := ps.OwnedTask("td-mid"); s.Status != "closed" {
		t.Errorf("td-mid status = %q once both children are done, want closed", s.Status)
	}
	// And the feature itself is left to its PR, never closed by the walk up.
	if s, ok, _ := ps.GetTask("os-feat"); !ok || s.Status != "open" {
		t.Errorf("os-feat status = %q, want open — the feature closes when its PR merges", s.Status)
	}
}

// TestCloseRefusesAParentWithOpenChildren: the same invariant on the human path. Discarding a tree
// is a different intent and keeps working, since scrap goes deepest-first.
func TestCloseRefusesAParentWithOpenChildren(t *testing.T) {
	e, ps := nestedFeature(t)
	err := e.CloseTask("repo", "td-mid")
	if err == nil {
		t.Fatal("closing a parent with open children must be refused")
	}
	for _, want := range []string{"td-deep1", "td-deep2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should name the open subtask %q: %v", want, err)
		}
	}
	if s, _, _ := ps.OwnedTask("td-mid"); s.Status != "open" {
		t.Errorf("td-mid status = %q after a refused close, want open", s.Status)
	}
	// A leaf still closes normally — the guard is about parents, not about closing.
	if err := e.CloseTask("repo", "td-flat"); err != nil {
		t.Errorf("closing a leaf must still work: %v", err)
	}
}
