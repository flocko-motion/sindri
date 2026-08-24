package agent

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/workflow"
)

// mountFor is the mode a role's pod gets at one container path, and whether it is mounted at all.
func mountFor(role, at string) (string, bool) {
	for _, m := range workspaceMounts(role, "/repo/.worktrees/dvalin", "/state/hidden", "/repo/.worktrees/brokk") {
		if m.Container == at {
			return m.Mode, true
		}
	}
	return "", false
}

// TestAWorkerAndReviewerGetTheirWorktreeAndNothingElse: the isolation the other two roles are
// exceptions TO. One workspace, read-write, and no overlay of any kind.
func TestAWorkerAndReviewerGetTheirWorktreeAndNothingElse(t *testing.T) {
	for _, role := range []string{"worker", "reviewer"} {
		got := workspaceMounts(role, "/repo/.worktrees/dvalin", "/state/hidden", "/repo/.worktrees/brokk")
		if len(got) != 1 {
			t.Fatalf("%s: %d workspace mounts, want just its worktree: %+v", role, len(got), got)
		}
		if got[0].Container != "/workspace" || got[0].Mode != "rw" || got[0].Host != "/repo/.worktrees/dvalin" {
			t.Errorf("%s: mount = %+v, want its own worktree at /workspace rw", role, got[0])
		}
	}
}

// TestAPlannerWritesOnlyOpenspec pins the older exception beside the new one, since both now come
// out of one function: a read-only workspace with openspec/ overlaid writable.
func TestAPlannerWritesOnlyOpenspec(t *testing.T) {
	if mode, ok := mountFor("planner", "/workspace"); !ok || mode != "ro" {
		t.Errorf("a planner's /workspace is %q (mounted=%v), want ro", mode, ok)
	}
	if mode, ok := mountFor("planner", "/workspace/openspec"); !ok || mode != "rw" {
		t.Errorf("a planner's openspec is %q (mounted=%v), want rw", mode, ok)
	}
}

// TestACoauthorCannotReachAnotherAgentsWorktree (sd-a6e45e): its /workspace is the user's own
// checkout, which holds every agent's tree — and the hub commits from those, so an edit made while
// looking around would land in another agent's PR as its work. The trees are covered by an empty
// read-only directory: not readable, not writable, and not even the wrong thing to read.
func TestACoauthorCannotReachAnotherAgentsWorktree(t *testing.T) {
	at := "/workspace/" + workflow.AgentTrees
	mode, ok := mountFor("coauthor", at)
	if !ok {
		t.Fatalf("%s is not covered for a coauthor — every agent's worktree is under it", at)
	}
	if mode != "ro" {
		t.Errorf("%s is mounted %q, want ro: nothing may ever be written into the cover", at, mode)
	}
	for _, m := range workspaceMounts("coauthor", "/repo", "/state/hidden", "/repo/.worktrees/brokk") {
		if m.Container == at && m.Host != "/state/hidden" {
			t.Errorf("%s is covered by %q, want the empty directory", at, m.Host)
		}
	}
	// And /workspace itself is still the user's checkout, read-write — that IS the role.
	if mode, ok := mountFor("coauthor", "/workspace"); !ok || mode != "rw" {
		t.Errorf("a coauthor's /workspace is %q (mounted=%v), want rw", mode, ok)
	}
}

// TestACoauthorGetsAScratchTree is the other half: it can no longer look at anyone's worktree, so it
// is given one of its own for the hub to check work out into (-> workflow.CmdScratch).
func TestACoauthorGetsAScratchTree(t *testing.T) {
	mode, ok := mountFor("coauthor", workflow.ScratchMount)
	if !ok {
		t.Fatalf("a coauthor has no %s — it could then inspect nothing but its own tree", workflow.ScratchMount)
	}
	if mode != "rw" {
		t.Errorf("%s is mounted %q, want rw: building and testing there is the point", workflow.ScratchMount, mode)
	}
	// No other role gets one: they each have a worktree of their own already.
	for _, role := range []string{"worker", "reviewer", "planner"} {
		if _, ok := mountFor(role, workflow.ScratchMount); ok {
			t.Errorf("%s was given a scratch tree", role)
		}
	}
}
