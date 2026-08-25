package workflow

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// splitTree seeds the shape that produced this: a feature with one subtask, that subtask already
// in_progress under another agent. An in-progress subtask is absent from every "is there work here"
// query, so the tree reads as finished from outside.
func splitTree(t *testing.T) (*Engine, *store.ProjectStore) {
	t.Helper()
	e, ps, _ := ownedEngine(t, "open")
	for _, name := range []string{"sudri", "dvalin"} {
		if err := ps.PutAgent(store.Agent{Name: name, Role: "worker"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := ps.UpsertTask(store.Task{ID: "sd-FEAT", Status: "open", Priority: "P2"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "sd-LEAF", Status: "in_progress", ParentID: "sd-FEAT", Priority: "P2"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "sd-LEAF", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	return e, ps
}

// TestAContainerIsNotOfferedWhileSomebodyIsInside is the prevention half. dvalin took sd-dfc0d5 as a
// leaf; a day later the hub handed sudri its PARENT, because an in-progress subtask is invisible to
// OpenSubtasks and to HasOpenDescendant alike, so the tree looked finished and free.
func TestAContainerIsNotOfferedWhileSomebodyIsInside(t *testing.T) {
	_, ps := splitTree(t)

	containers, err := ps.OpenContainers()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range containers {
		if c.ID == "sd-FEAT" {
			t.Fatal("a feature was offered while another agent was working inside it")
		}
	}

	// Once its holder lets go, the tree is free again — the guard is about occupancy, not the shape.
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	containers, err = ps.OpenContainers()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range containers {
		found = found || c.ID == "sd-FEAT"
	}
	if !found {
		t.Error("with nobody inside it, the feature must be claimable again")
	}
}

// TestASplitHierarchyHealsItself is the other half: prevention cannot cover a tree SPLIT after the
// fact, since reparenting a task under a held feature makes two agents out of one and no claim was
// made to refuse. The CONTAINER holder yields — the leaf is concrete work in progress.
func TestASplitHierarchyHealsItself(t *testing.T) {
	e, ps := splitTree(t)
	// The split, as a reparenting leaves it: sudri holding the feature dvalin is inside.
	if err := ps.SetState(store.AgentState{Agent: "sudri", Container: "sd-FEAT", Phase: "working"}); err != nil {
		t.Fatal(err)
	}

	if !e.healSplit("proj", "sudri") {
		t.Fatal("the split was not noticed")
	}
	if st, _ := ps.GetState("sudri"); st.Container != "" {
		t.Errorf("sudri still holds %q — the container holder is the one that yields", st.Container)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "sd-LEAF" {
		t.Errorf("dvalin lost its work: %+v", st)
	}
	// Told, and told WHY: "you no longer hold it" without a reason reads as work being taken away.
	deps := e.deps.(*stubDeps)
	if len(deps.injectedText) == 0 || !strings.Contains(deps.injectedText[len(deps.injectedText)-1], "dvalin") {
		t.Errorf("the released agent must be told who is inside it, got %v", deps.injectedText)
	}
}

// TestHealingLeavesAnUndividedFeatureAlone: the ordinary case is one agent working its own tree, and
// a sweep that released it would take a feature off whoever is correctly holding it.
func TestHealingLeavesAnUndividedFeatureAlone(t *testing.T) {
	e, ps := splitTree(t)
	// dvalin holds BOTH the feature and the subtask under it — the normal feature flow.
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "sd-LEAF", Container: "sd-FEAT", Phase: "working"}); err != nil {
		t.Fatal(err)
	}

	if e.healSplit("proj", "dvalin") {
		t.Error("an agent working inside its own feature was released from it")
	}
	if st, _ := ps.GetState("dvalin"); st.Container != "sd-FEAT" {
		t.Errorf("dvalin lost its own feature: %+v", st)
	}
}
