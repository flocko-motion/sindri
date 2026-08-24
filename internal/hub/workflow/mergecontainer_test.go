package workflow

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestAMilestoneConflictKeepsTheContainer is the shape austri found (sd-5ef393): the pre-merge
// rebase conflicting sent a feature worker to "resolving" via a SetState that named the whole row
// from PR fields — Task became the PR's, which for a milestone is the CONTAINER's id, and Container
// itself was dropped, unhooking the agent from its feature for the whole resolution.
func TestAMilestoneConflictKeepsTheContainer(t *testing.T) {
	e, ps, _, deps := featureWorker(t, true) // td-1 still open, held on td-EPIC
	wt := filepath.Join(deps.root, ".worktrees", "dain")
	commitIn(t, wt, "shared.txt", "the PR's line\n", "PR edits shared")
	commitIn(t, deps.root, "shared.txt", "base's line\n", "base edits shared")
	if err := ps.PutPR(store.PR{
		ID: "pr-td-EPIC", Task: "td-EPIC", Agent: "dain", Branch: "td-EPIC", Base: "main", Status: "approved", Kind: "interim",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.Merge("repo", "pr-td-EPIC"); err == nil {
		t.Fatal("a conflicting rebase must be reported as an error, not merged")
	}

	st, _ := ps.GetState("dain")
	if st.Container != "td-EPIC" || st.Task != "td-1" || st.Branch != "td-EPIC" || st.Phase != "resolving" {
		t.Errorf("a milestone conflict must leave {container:td-EPIC task:td-1 branch:td-EPIC phase:resolving}, got {container:%q task:%q branch:%q phase:%q}",
			st.Container, st.Task, st.Branch, st.Phase)
	}
}

// TestAnInterimConflictKeepsItsTask is the same hazard on a plain leaf task: the pre-merge conflict
// path must not overwrite the held task or branch with the PR's own fields — here they agree, but
// only because nothing else could have changed them; the fix stops assuming that and touches phase
// alone.
func TestAnInterimConflictKeepsItsTask(t *testing.T) {
	e, ps, _, deps := leafWorker(t, "submitted")
	wt := filepath.Join(deps.root, ".worktrees", "dain")
	commitIn(t, wt, "seed", "the PR's line\n", "PR edits seed")
	commitIn(t, deps.root, "seed", "base's line\n", "base edits seed")
	if err := ps.PutPR(store.PR{
		ID: "pr-td-LEAF", Task: "td-LEAF", Agent: "dain", Branch: "td-LEAF", Base: "main", Status: "approved", Kind: "interim",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.Merge("repo", "pr-td-LEAF"); err == nil {
		t.Fatal("a conflicting rebase must be reported as an error, not merged")
	}

	st, _ := ps.GetState("dain")
	if st.Container != "" || st.Task != "td-LEAF" || st.Branch != "td-LEAF" || st.Phase != "resolving" {
		t.Errorf("a leaf conflict must leave {task:td-LEAF branch:td-LEAF phase:resolving}, got {container:%q task:%q branch:%q phase:%q}",
			st.Container, st.Task, st.Branch, st.Phase)
	}
}

// TestFinishingAnInterimContributionOnlyChangesPhase guards finishPartialMerge's non-feature arm:
// by construction (promoteToFeature only touches a "working" phase) an interim PR's agent never
// picks up a container while its PR is out, so the fix — SetPhase instead of a whole-row write —
// must land the agent back on exactly the task and branch it already held.
func TestFinishingAnInterimContributionOnlyChangesPhase(t *testing.T) {
	e, ps, _, _ := leafWorker(t, "submitted")
	if err := ps.PutPR(store.PR{
		ID: "pr-td-LEAF", Task: "td-LEAF", Agent: "dain", Branch: "td-LEAF", Base: "main", Status: "approved", Kind: "interim",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.Merge("repo", "pr-td-LEAF"); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	st, _ := ps.GetState("dain")
	if st.Container != "" || st.Task != "td-LEAF" || st.Branch != "td-LEAF" || st.Phase != "working" {
		t.Errorf("an interim merge should resume {task:td-LEAF branch:td-LEAF phase:working}, got {container:%q task:%q branch:%q phase:%q}",
			st.Container, st.Task, st.Branch, st.Phase)
	}
}
