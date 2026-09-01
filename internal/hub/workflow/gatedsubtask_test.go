package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatalf("set state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".worktrees", agent, "feature.txt"), []byte("built\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return newEngine(st, &stubDeps{root: root}), ps, registry.Caller{Project: "repo", Agent: agent, Role: "worker", Phase: "working"}
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
// "finished". With nothing workable and nothing finished, `sindri` answers at once with a truthful
// wait rather than declaring the feature done, and the user's verdict is what actually releases it.
func TestAGatedFeatureWaitsRatherThanBeingDeclaredDone(t *testing.T) {
	e, ps, _ := gatedFeature(t)
	// Every subtask that CAN be worked is done, which is the state a checkpoint leaves behind.
	if err := e.SetStatus("repo", "td-1", "closed"); err != nil {
		t.Fatal(err)
	}
	if err := e.RefreshTask("repo", "td-1"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dain", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}

	d, err := e.AgentDirective(t.Context(), "repo", "dain")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(d, "td-2") || !strings.Contains(d, "isn't finished") {
		t.Errorf("with nothing workable and nothing finished, the directive should say so, got: %s", d)
	}

	// The verdict releases it, and the next ask hands over the subtask.
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

// TestARejectedSubtaskBlocksNothing: a rejection is a verdict already given, so it cannot hold a
// feature open. Nothing would ever clear it — the wait ends when the user rules, and on a rejected
// task they have — so blocking would park the holder indefinitely and silently, which is a worse
// failure than the wrong completion this guard exists to prevent.
func TestARejectedSubtaskBlocksNothing(t *testing.T) {
	e, ps, c := gatedFeature(t)
	if err := ps.SetApproval("td-2", "rejected", "not wanted after all"); err != nil {
		t.Fatal(err)
	}
	if gated, err := e.gatedUnder("repo", "td-EPIC"); err != nil || len(gated) != 0 {
		t.Fatalf("a rejected subtask must not hold the feature open, got %v (err=%v)", openIDs(gated), err)
	}
	var out strings.Builder
	if code, err := e.CmdCheckpoint(c, []string{"the worked one"}, &out); err != nil || code != 0 {
		t.Fatalf("CmdCheckpoint: code=%d err=%v out=%s", code, err, out.String())
	}
	if !strings.Contains(out.String(), "submit") {
		t.Errorf("with only a rejected subtask left the feature is finished:\n%s", out.String())
	}
	// And the directive agrees rather than waiting for a verdict that has already been given.
	d, err := e.AgentDirective(t.Context(), "repo", "dain")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(d, "submit") {
		t.Errorf("the held feature should be finished, got: %s", d)
	}
}

// TestEditTellsTheHolderOfTheEnclosingFeature is the case that actually bites and that matching the
// edited ROW missed entirely: nobody holds td-2, so nobody was told, while the worker holding the
// feature it belongs to had just had its unit of work changed. It is told for a direct subtask and
// for one further down, since a feature contains its whole tree.
func TestEditTellsTheHolderOfTheEnclosingFeature(t *testing.T) {
	for _, depth := range []string{"child", "grandchild"} {
		e, ps, _ := gatedFeature(t)
		deps := e.deps.(*stubDeps)
		deps.alive = true
		if err := ps.PutAgent(store.Agent{Name: "dain", Role: "worker"}); err != nil {
			t.Fatal(err)
		}
		if depth == "grandchild" {
			if err := ps.UpsertTask(store.Task{ID: "td-MID", Title: "an epic between", Status: "open", ParentID: "td-EPIC"}); err != nil {
				t.Fatal(err)
			}
			if err := ps.SetParent("td-MID", "td-EPIC"); err != nil {
				t.Fatal(err)
			}
			if err := ps.SetParent("td-2", "td-MID"); err != nil {
				t.Fatal(err)
			}
		}

		planner := registry.Caller{Project: "repo", Agent: "galar", Role: "planner"}
		var out strings.Builder
		if code, err := e.CmdEditTask(planner, []string{"td-2", "--body", "the premise was wrong"}, &out); code != 0 || err != nil {
			t.Fatalf("%s: edit: code=%d err=%v out=%s", depth, code, err, out.String())
		}
		if len(deps.injected) != 1 || deps.injected[0] != "dain" {
			t.Fatalf("%s: the holder of the enclosing feature must be told, injected: %v", depth, deps.injected)
		}
		// It has to be able to judge whether this touches what it is building: which task changed,
		// how, that it sits inside the feature it holds, and that it is not the task it is on.
		for _, want := range []string{"td-2", "description", "td-EPIC", "not the task you're on", "sindri task td-2"} {
			if !strings.Contains(deps.injectedText[0], want) {
				t.Errorf("%s: the note should carry %q:\n%s", depth, want, deps.injectedText[0])
			}
		}
		if !strings.Contains(out.String(), "dain holds td-EPIC") {
			t.Errorf("%s: the planner should be told whose work this reached:\n%s", depth, out.String())
		}
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
