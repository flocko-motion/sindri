package workflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// plannerWithTask seeds a planner and one task the user wrote, and returns the engine, the store,
// the stub that records what the planner was told, and the task id.
func plannerWithTask(t *testing.T) (*Engine, *store.ProjectStore, *stubDeps, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "galar", Role: "planner", Workspace: ".worktrees/galar"}); err != nil {
		t.Fatalf("put planner: %v", err)
	}
	const id = "td-abc123"
	if err := ps.PutOwnedTask(store.OwnedTask{
		ID: id, Title: "a query API", Status: "open", Priority: "P2", Description: "the body the user wrote",
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	deps := &stubDeps{root: root}
	return newEngine(st, deps), ps, deps, id
}

// TestHandingATaskOverOpensTheApprovalGate is what makes the workflow possible at all: edit-task is
// pending-only, so a task the user wrote could never be revised by the planner working it up. The
// same flag holds it back from workers while it is in flux and returns it for a verdict when done.
func TestHandingATaskOverOpensTheApprovalGate(t *testing.T) {
	e, ps, _, id := plannerWithTask(t)
	if appr, _ := ps.GetApproval(id); appr != "" {
		t.Fatalf("a task the user wrote starts ungated, got %q", appr)
	}
	if err := e.AssignPlan("proj", "galar", "", id); err != nil {
		t.Fatalf("AssignPlan: %v", err)
	}
	if appr, _ := ps.GetApproval(id); appr != "pending" {
		t.Errorf("approval = %q, want pending — the planner cannot revise it otherwise", appr)
	}
}

// TestClosingATaskClosesItsGate: the gate the hand-over opens must not outlive the task. The board
// reads the gate over the status, so a closed task whose gate still said "pending" rendered as
// pending — the close looked as though it had done nothing, however often it was repeated.
func TestClosingATaskClosesItsGate(t *testing.T) {
	for _, scrap := range []bool{false, true} {
		e, ps, _, id := plannerWithTask(t)
		if err := e.AssignPlan("proj", "galar", "", id); err != nil {
			t.Fatalf("AssignPlan: %v", err)
		}
		if err := e.finishTask("proj", id, scrap); err != nil {
			t.Fatalf("finishTask(scrap=%v): %v", scrap, err)
		}
		if appr, _ := ps.GetApproval(id); appr != "" {
			t.Errorf("scrap=%v: approval = %q, want it gone with the task", scrap, appr)
		}
	}
}

// TestTheTaskCarriesTheBrief: the point of handing over a task rather than retyping it. Its title and
// body reach the planner, and the brief names it as the parent everything hangs under.
func TestTheTaskCarriesTheBrief(t *testing.T) {
	e, _, deps, id := plannerWithTask(t)
	if err := e.AssignPlan("proj", "galar", "", id); err != nil {
		t.Fatalf("AssignPlan: %v", err)
	}
	if len(deps.injectedText) != 1 {
		t.Fatalf("want one brief injected, got %d", len(deps.injectedText))
	}
	brief := deps.injectedText[0]
	for _, want := range []string{id, "a query API", "the body the user wrote", "--parent " + id} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief must carry %q:\n%s", want, brief)
		}
	}
	// The interview phases still apply — working up a task is not a licence to draft.
	for _, want := range []string{"INTERVIEW", "PHASE 1", GoToken} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief must still impose %q", want)
		}
	}
}

// TestFreeTextPlanIsUnchanged: handing over a task is an addition, so a plan with no task keeps its
// wording and gains no parent instruction to follow.
func TestFreeTextPlanIsUnchanged(t *testing.T) {
	e, _, deps, _ := plannerWithTask(t)
	if err := e.AssignPlan("proj", "galar", "work out a query API", ""); err != nil {
		t.Fatalf("AssignPlan: %v", err)
	}
	brief := deps.injectedText[0]
	if !strings.Contains(brief, "PLAN THIS: work out a query API") {
		t.Errorf("free-text plans keep their opening:\n%s", brief)
	}
	// PHASE 4 mentions --parent as general guidance; what must be absent is a parent to hang
	// things under, since there is no task here to be one.
	if strings.Contains(brief, "WORK UP TASK") {
		t.Errorf("with no task the brief must not open as a task hand-over:\n%s", brief)
	}
}

// TestExtraTextAccompaniesTheTask: the task is the brief, and the textarea is for what it omits.
func TestExtraTextAccompaniesTheTask(t *testing.T) {
	e, _, deps, id := plannerWithTask(t)
	if err := e.AssignPlan("proj", "galar", "start from the read path", id); err != nil {
		t.Fatalf("AssignPlan: %v", err)
	}
	brief := deps.injectedText[0]
	for _, want := range []string{"a query API", "start from the read path"} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief must carry %q:\n%s", want, brief)
		}
	}
}

// TestPlanRefusesATaskItDoesNotOwn: a planner works up sindri's own tasks. A GitHub issue or an
// openspec change has an upstream that owns its text, so revising it here would diverge silently.
func TestPlanRefusesATaskItDoesNotOwn(t *testing.T) {
	e, _, _, _ := plannerWithTask(t)
	err := e.AssignPlan("proj", "galar", "", "gh-42")
	if err == nil {
		t.Fatal("handing over a task the project does not own must be refused")
	}
	if !strings.Contains(err.Error(), "gh-42") {
		t.Errorf("the refusal should name the id: %v", err)
	}
}

// TestPlanStillNeedsASubject: neither a task nor a goal leaves nothing to plan.
func TestPlanStillNeedsASubject(t *testing.T) {
	e, _, _, _ := plannerWithTask(t)
	if err := e.AssignPlan("proj", "galar", "  ", ""); err == nil {
		t.Error("an empty plan assignment must be refused")
	}
}
