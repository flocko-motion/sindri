package workflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// gatedFeature is the tree a planner edit produces mid-flight: a worker holds feature td-EPIC and is
// working subtask td-1, while its sibling td-2 has just been edited and so awaits the user again.
// Both subtasks are real work; only one of them can be handed out.
func gatedFeature(t *testing.T) (*Engine, *store.ProjectStore, registry.Caller) {
	t.Helper()
	const agent = "dain"
	root, _ := newWorkRepo(t, agent, "td-EPIC")
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: filepath.Join(".worktrees", agent)}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	for _, x := range []store.Task{
		{ID: "td-EPIC", Title: "a feature", Status: "open", Priority: "P1", Type: "epic"},
		{ID: "td-1", Title: "the one being worked", Status: "open", Priority: "P1", ParentID: "td-EPIC"},
		{ID: "td-2", Title: "the edited sibling", Status: "open", Priority: "P1", ParentID: "td-EPIC"},
	} {
		if err := ps.UpsertTask(x); err != nil {
			t.Fatalf("seed %s: %v", x.ID, err)
		}
		if x.ID != "td-EPIC" {
			if err := ps.PutOwnedTask(store.OwnedTask{ID: x.ID, Title: x.Title, Status: "open"}); err != nil {
				t.Fatalf("own %s: %v", x.ID, err)
			}
			if err := ps.SetParent(x.ID, "td-EPIC"); err != nil {
				t.Fatalf("parent %s: %v", x.ID, err)
			}
		}
	}
	// The edit's consequence, which is the whole point: td-2 is back with the user.
	if err := ps.SetApproval("td-2", "pending", ""); err != nil {
		t.Fatalf("gate td-2: %v", err)
	}
	if err := ps.SetState(store.AgentState{
		Agent: agent, Container: "td-EPIC", Branch: "td-EPIC", Task: "td-1", Phase: "working",
	}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".worktrees", agent, "feature.txt"), []byte("built\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return New(st, &stubDeps{root: root}), ps, registry.Caller{Project: "repo", Agent: agent, Role: "worker", Phase: "working"}
}

// TestAFeatureIsNotFinishedOverGatedWork is the defect an edit inside a held feature would otherwise
// cause, end to end. OpenSubtasks answers "what is workable now", so a gated subtask is not withheld
// from it — it is ABSENT, which reads exactly like finished. The worker was told the feature was
// complete, submitted the branch, and the feature closed with a subtask nobody had worked.
func TestAFeatureIsNotFinishedOverGatedWork(t *testing.T) {
	e, ps, c := gatedFeature(t)

	// Checkpointing the last workable subtask must not announce a finished feature.
	var out strings.Builder
	if code, err := e.CmdCheckpoint(c, []string{"the worked one"}, &out); err != nil || code != 0 {
		t.Fatalf("CmdCheckpoint: code=%d err=%v out=%s", code, err, out.String())
	}
	for _, want := range []string{"td-2", "isn't finished", "awaiting the user's approval"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the checkpoint reply should say why the feature can't go up (%q):\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "submit") {
		t.Errorf("a feature with gated work under it must not be sent to submit:\n%s", out.String())
	}

	// And the gate holds even if it submits anyway: the PR is what would carry the feature away.
	out.Reset()
	if code, _ := e.CmdSubmit(c, []string{"the whole feature"}, &out); code == 0 {
		t.Error("a feature with gated work under it must not go up")
	}
	if _, ok, _ := ps.GetPR("pr-td-EPIC"); ok {
		t.Error("no PR should exist for a feature that still has work awaiting the user")
	}
	if !strings.Contains(out.String(), "td-2") {
		t.Errorf("the refusal should name the work holding it open:\n%s", out.String())
	}
}

// TestAGatedFeatureWaitsRatherThanBeingDeclaredDone: the directive is the other route to a false
// "finished". With nothing workable and nothing finished, there is nothing to say — so `sindri`
// waits, exactly as it does for any other empty queue, and the user's verdict releases it.
func TestAGatedFeatureWaitsRatherThanBeingDeclaredDone(t *testing.T) {
	e, ps, _ := gatedFeature(t)
	// Every subtask that CAN be worked is done, which is the state a checkpoint leaves behind.
	if err := e.SetStatus("repo", "td-1", "closed"); err != nil {
		t.Fatal(err)
	}
	if err := e.RefreshTask("repo", "td-1"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dain", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	d, err := e.AgentDirective(ctx, "repo", "dain")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the directive should wait on the user, got %q (err=%v)", d, err)
	}

	// The verdict releases it, and the same call then hands over the subtask.
	if err := e.ApproveTask("repo", "td-2", false); err != nil {
		t.Fatal(err)
	}
	d, err = e.AgentDirective(t.Context(), "repo", "dain")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(d, "td-2") {
		t.Errorf("once approved, the held feature should hand over td-2, got: %s", d)
	}
	if s, _ := ps.GetState("dain"); s.Task != "td-2" || s.Phase != "working" {
		t.Errorf("state = {task:%q phase:%q}, want {td-2 working}", s.Task, s.Phase)
	}
}

// TestGatedUnderReachesAnyDepth: the completion question has to reach as far as the assignment one
// does. OpenSubtasks walks the whole tree, so a gated grandchild vanishes from it just as a gated
// child does — and the status reconciler, which reads DIRECT children only, catches neither in time.
func TestGatedUnderReachesAnyDepth(t *testing.T) {
	e, ps, _ := gatedFeature(t)
	// Re-hang the gated subtask one level further down, under a mid-level epic.
	for _, x := range []store.Task{{ID: "td-MID", Title: "an epic in between", Status: "open", ParentID: "td-EPIC"}} {
		if err := ps.UpsertTask(x); err != nil {
			t.Fatal(err)
		}
	}
	if err := ps.SetParent("td-2", "td-MID"); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-2", Title: "the edited sibling", Status: "open", Priority: "P1", ParentID: "td-MID"}); err != nil {
		t.Fatal(err)
	}
	gated, err := e.gatedUnder("repo", "td-EPIC")
	if err != nil {
		t.Fatalf("gatedUnder: %v", err)
	}
	if len(gated) != 1 || gated[0].ID != "td-2" {
		t.Errorf("a gated grandchild must hold the feature open too, got %v", openIDs(gated))
	}
}
