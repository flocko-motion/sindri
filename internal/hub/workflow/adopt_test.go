package workflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// leafWorker seeds a worker mid-flight on ONE task, held as a leaf — the state a child is added
// into. A real worktree, so submit can reach its own guard rather than dying on git first.
func leafWorker(t *testing.T, phase string) (*Engine, *store.ProjectStore, registry.Caller, *stubDeps) {
	t.Helper()
	const agent = "dain"
	root, _ := newWorkRepo(t, agent, "td-LEAF")
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: filepath.Join(".worktrees", agent)}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-LEAF", Title: "one task", Status: "in_progress", Priority: "P1"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-LEAF", Title: "one task", Status: "in_progress"}); err != nil {
		t.Fatalf("own: %v", err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Task: "td-LEAF", Branch: "td-LEAF", Phase: phase}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	deps := &stubDeps{root: root, alive: true}
	return New(st, deps), ps, registry.Caller{Project: "repo", Agent: agent, Role: "worker", Phase: phase}, deps
}

// gatedFeatureAlive is gatedFeature with its worker reachable, for the cases that assert what it
// was told. Its td-2 stays where gatedFeature left it — pending, and beside the point here.
func gatedFeatureAlive(t *testing.T) (*Engine, *store.ProjectStore, registry.Caller, *stubDeps) {
	t.Helper()
	e, ps, c := gatedFeature(t)
	deps := e.deps.(*stubDeps)
	deps.alive = true
	if err := ps.PutAgent(store.Agent{Name: "dain", Role: "worker", Workspace: filepath.Join(".worktrees", "dain")}); err != nil {
		t.Fatal(err)
	}
	return e, ps, c, deps
}

// addChildTo proposes an approved child under parent and returns its id.
func addChildTo(t *testing.T, e *Engine, project, parent string) string {
	t.Helper()
	id, err := e.CreateTask(project, TaskSpec{Title: "the work it gained", Parent: parent})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	return id
}

// addChild proposes a child under parent the way a planner does, and returns its id.
func addChild(t *testing.T, e *Engine, parent string, approved bool) string {
	t.Helper()
	id, err := e.CreateTask("repo", TaskSpec{Title: "the work it gained", Parent: parent})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if !approved {
		if err := e.store.For("repo").SetApproval(id, "pending", ""); err != nil {
			t.Fatalf("gate child: %v", err)
		}
	}
	return id
}

// TestATaskThatGainsAChildBecomesAFeatureItsWorkerKeeps: the agent's unit of work grew, so it takes
// the new work on rather than being stranded in front of it or merging over it. Same agent, same
// branch — a leaf branch is already named for its task — and one PR at the end.
func TestATaskThatGainsAChildBecomesAFeatureItsWorkerKeeps(t *testing.T) {
	e, ps, _, deps := leafWorker(t, "working")
	child := addChild(t, e, "td-LEAF", true)

	st, _ := ps.GetState("dain")
	if st.Container != "td-LEAF" || st.Branch != "td-LEAF" {
		t.Fatalf("the task should be held as a feature on the same branch, got {container:%q branch:%q}", st.Container, st.Branch)
	}
	// The subtask comes from the ordinary directive, which is what asks every question about what
	// may be handed out — so the promotion itself assigns nothing.
	d, err := e.AgentDirective(t.Context(), "repo", "dain")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(d, child) {
		t.Errorf("the feature should hand over the work it gained, got: %s", d)
	}
	if s, _ := ps.GetState("dain"); s.Task != child || s.Phase != "working" {
		t.Errorf("the agent should be working the new child, got {task:%q phase:%q}", s.Task, s.Phase)
	}
	// And told, because being refused at the next checkpoint is not how it should find out.
	if len(deps.injected) != 1 || deps.injected[0] != "dain" {
		t.Fatalf("the worker should be told its unit grew, injected: %v", deps.injected)
	}
	for _, want := range []string{child, "td-LEAF", "FEATURE", "ONE pull request", "sindri task " + child} {
		if !strings.Contains(deps.injectedText[0], want) {
			t.Errorf("the note should carry %q:\n%s", want, deps.injectedText[0])
		}
	}
}

// TestAGainedChildAwaitingAVerdictParksRatherThanIsHandedOut: promotion must not become a route
// round the approval gate. The agent holds the feature and waits, which is the ordinary answer for
// a feature with nothing workable under it.
func TestAGainedChildAwaitingAVerdictParksRatherThanIsHandedOut(t *testing.T) {
	e, ps, _, _ := leafWorker(t, "working")
	child := addChild(t, e, "td-LEAF", false)

	st, _ := ps.GetState("dain")
	if st.Container != "td-LEAF" {
		t.Fatalf("it is still a feature, got container %q", st.Container)
	}
	if st.Task == child {
		t.Errorf("a child awaiting the user's verdict must not be handed out, got task %q", st.Task)
	}
	if gated, err := e.gatedUnder("repo", "td-LEAF"); err != nil || len(gated) != 1 {
		t.Errorf("the feature should be held open by the pending child, got %v (err=%v)", openIDs(gated), err)
	}
}

// TestSubmitOfAGrownTaskExtendsItRatherThanRefusing is the same growth reached by the other door —
// the child arrived while the agent was down, so nothing promoted it at the time. A bare refusal
// would leave it with a task it cannot finish and no verb that reaches the child.
func TestSubmitOfAGrownTaskExtendsItRatherThanRefusing(t *testing.T) {
	e, ps, c, deps := leafWorker(t, "working")
	deps.alive = false // nobody to inject into, so the promotion at add-time does not happen
	child := addChild(t, e, "td-LEAF", true)
	if err := ps.SetState(store.AgentState{Agent: "dain", Task: "td-LEAF", Branch: "td-LEAF", Phase: "working"}); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	code, err := e.CmdSubmit(c, []string{"done"}, &out)
	if err != nil {
		t.Fatalf("CmdSubmit: %v", err)
	}
	if code == 0 {
		t.Error("a task with open work under it must not go up as finished")
	}
	if _, ok, _ := ps.GetPR("pr-td-LEAF"); ok {
		t.Error("no PR should exist for a task whose children are still open")
	}
	for _, want := range []string{child, "FEATURE", "same branch"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the reply should carry %q:\n%s", want, out.String())
		}
	}
	// Extended, not merely refused: it holds the feature, and the directive hands it the new work.
	if st, _ := ps.GetState("dain"); st.Container != "td-LEAF" {
		t.Errorf("the agent should be holding the feature, got container %q", st.Container)
	}
	d, derr := e.AgentDirective(t.Context(), "repo", "dain")
	if derr != nil || !strings.Contains(d, child) {
		t.Errorf("the directive should hand over the work it gained, got %q (err=%v)", d, derr)
	}
}

// TestASubtaskThatGainsAChildStaysInsideItsFeature is the nesting edge, decided rather than
// discovered: one level down needs no promotion at all. The subtask's child is inside the same
// feature on the same branch, so the feature loop already reaches it — it serves leaves at any
// depth — and the agent keeps the state it has. It is still told, since its unit grew.
func TestASubtaskThatGainsAChildStaysInsideItsFeature(t *testing.T) {
	e, ps, c, deps := gatedFeatureAlive(t)
	child := addChildTo(t, e, "repo", "td-1")

	st, _ := ps.GetState("dain")
	if st.Container != "td-EPIC" || st.Task != "td-1" {
		t.Errorf("a worker inside a feature keeps its state, got {container:%q task:%q}", st.Container, st.Task)
	}
	if len(deps.injected) != 1 || !strings.Contains(deps.injectedText[0], child) {
		t.Fatalf("it should still be told what its unit gained, injected: %v", deps.injectedText)
	}
	if !strings.Contains(deps.injectedText[0], "Carry on with the subtask you are on") {
		t.Errorf("nothing changed about what it is doing, so the note should say so:\n%s", deps.injectedText[0])
	}
	// And the checkpoint carries it on to the new work instead of refusing over it.
	var out strings.Builder
	if code, err := e.CmdCheckpoint(c, []string{"the parent's own part"}, &out); code != 0 || err != nil {
		t.Fatalf("CmdCheckpoint: code=%d err=%v out=%s", code, err, out.String())
	}
	if tk, _, _ := ps.OwnedTask("td-1"); tk.Status == "closed" {
		t.Error("td-1 must not be closed over the child it gained")
	}
	if s, _ := ps.GetState("dain"); s.Task != child {
		t.Errorf("the agent should be on the work it gained, got %q", s.Task)
	}
}

// TestAMergeNeverClosesATaskOverOpenChildren is the invariant everything here traces back to. A PR
// out when the child arrives is not moved — this is the guard that catches it instead — so the
// merge lands the branch as a MILESTONE and leaves the task, and its worker, on the feature.
func TestAMergeNeverClosesATaskOverOpenChildren(t *testing.T) {
	e, ps, c, deps := leafWorker(t, "working")
	var out strings.Builder
	if code, err := e.CmdSubmit(c, []string{"the leaf"}, &out); code != 0 || err != nil {
		t.Fatalf("CmdSubmit: code=%d err=%v out=%s", code, err, out.String())
	}
	deps.alive = false
	child := addChild(t, e, "td-LEAF", true)
	if st, _ := ps.GetState("dain"); st.Container != "" {
		t.Fatalf("a PR already out must not be moved, got container %q", st.Container)
	}

	pr, _, _ := ps.GetPR("pr-td-LEAF")
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Merge("repo", "pr-td-LEAF"); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if tk, _, _ := ps.OwnedTask("td-LEAF"); tk.Status == "closed" {
		t.Error("the merge closed a task over work still open beneath it")
	}
	st, _ := ps.GetState("dain")
	if st.Container != "td-LEAF" || st.Task != child {
		t.Errorf("the merge should leave the worker on the feature and its new work, got {container:%q task:%q}", st.Container, st.Task)
	}
}
