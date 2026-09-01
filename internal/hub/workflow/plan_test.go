package workflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestPlanAssignmentGatesEveryFailureMode: the brief exists because a free-text "plan X" produced
// a planner that read nothing, asked nothing, and specified what the codebase already had. Each
// of those has to be answered explicitly, in order, or the brief is just more prose to skim.
func TestPlanAssignmentGatesEveryFailureMode(t *testing.T) {
	msg := MsgPlanAssignment("a query API", "", "docs/ARCH.md", "/workspace/papers/ranke.pdf")

	for _, want := range []string{
		"PLAN THIS: a query API", // the assignment is restated, not assumed
		"INTERVIEW",              // and it is a conversation, not freestyle drafting
		"/workspace/docs/ARCH.md",
		"/workspace/papers/ranke.pdf", // the material it could not have guessed
		"already exist",               // the check that would have saved the wasted work
		"SAY SO AND STOP",             // …and that stopping is a success
		"ONE question at a time",      // interview, not a form
		"never answer your own",       // the way an agent avoids asking
		GoToken,                       // nothing is written without it
		"openspec submit",             // where it ends up
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the brief must contain %q:\n%s", want, msg)
		}
	}
	// The phases have to be ordered, or "do the reading" is advice rather than a gate.
	if i, j := strings.Index(msg, "PHASE 1"), strings.Index(msg, "PHASE 4"); i < 0 || j < 0 || i > j {
		t.Error("phases must appear in order")
	}
}

// TestPlanAssignmentHandlesDivergence: when the code does not match how the user described it,
// the dangerous outcome is a spec that quietly accommodates the difference — the bug is then
// hidden AND built on. So the brief has to demand verification first (a misreading costs more
// than a question), then disclosure, and treat the finding as a success whose fix is part of the
// job rather than an obstacle to the plan.
func TestPlanAssignmentHandlesDivergence(t *testing.T) {
	msg := MsgPlanAssignment("a query API", "", "docs/ARCH.md", "")
	for _, want := range []string{
		"does the code match how the user described it", // look for it at all
		"VERIFY",              // before accusing
		"may have misread",    // why verification comes first
		"named plainly",       // disclosure
		"NEVER design around", // the failure mode being forbidden
		"hides it",            // and why it is the dangerous one
		"SUCCESS",             // finding one is not a setback
		"propose it as its own task",
		"NAME it in the plan", // the floor, when a task is too much
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the brief must contain %q:\n%s", want, msg)
		}
	}
}

// TestGoRuleRejectsThePolitePhrases: agreement is not authorisation. These are the exact things a
// user says while still thinking, and treating one as GO restarts the whole problem.
func TestGoRuleRejectsThePolitePhrases(t *testing.T) {
	for _, phrase := range []string{"go ahead", "do it", "sounds good", "yes"} {
		if !strings.Contains(GoRule, phrase) {
			t.Errorf("the rule should name %q as NOT approval:\n%s", phrase, GoRule)
		}
	}
	if !strings.Contains(GoRule, "ask for it") {
		t.Error("an agent that thinks it has approval should be told to ask")
	}
}

// TestPlanAssignmentDropsUnconfiguredReading: a project with no reading list gets no dangling
// phrase about material it does not have.
func TestPlanAssignmentDropsUnconfiguredReading(t *testing.T) {
	msg := MsgPlanAssignment("something", "", "", "")
	if strings.Contains(msg, "plans against") {
		t.Errorf("no reading configured should mean no reading line:\n%s", msg)
	}
	if strings.Contains(msg, "/workspace/\n") {
		t.Errorf("a bare /workspace/ means an empty path leaked in:\n%s", msg)
	}
}

// TestPlanAssignmentOffersTaskOnlyPath: Phase 4 used to present one unconditional sequence —
// draft a spec, then submit it — which left "just create the backlog tasks, no spec needed" out
// of the brief entirely, even though create-task and --parent hierarchies already worked. The
// brief must now name that as a first-class outcome, alongside the spec path, and hand it the
// vocabulary (--parent, --type epic) a multi-piece backlog needs.
func TestPlanAssignmentOffersTaskOnlyPath(t *testing.T) {
	msg := MsgPlanAssignment("a query API", "", "docs/ARCH.md", "")
	for _, want := range []string{
		"just backlog work, nothing worth writing down",
		"--parent",
		"--type epic",
		"sindri state idle",
		"No spec, no PR",
		"work with a design worth recording",
		"openspec submit", // the spec path still ends here when the interview calls for one
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the brief must offer the task-only path, missing %q:\n%s", want, msg)
		}
	}
}

// TestAssignPlanRefusedWithAnOpenPR: a planner drafts on ONE standing branch, so a second plan
// would pile unreviewed work onto specs still awaiting a verdict — and merging that PR would land
// two decisions the user agreed to one of.
func TestAssignPlanRefusedWithAnOpenPR(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "galar", Role: "planner", Workspace: "."}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-os-new", Agent: "galar", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	e := newEngine(st, &stubDeps{root: root, alive: true})

	err = e.AssignPlan("proj", "galar", "another thing", "")
	if err == nil {
		t.Fatal("expected a refusal while a PR is open")
	}
	for _, want := range []string{"pr-os-new", "merge", "scrap"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should name %q so the user can act: %v", want, err)
		}
	}

	// Once that PR is out of the way, the assignment goes through.
	if err := ps.PutPR(store.PR{ID: "pr-os-new", Agent: "galar", Status: "merged"}); err != nil {
		t.Fatal(err)
	}
	if err := e.AssignPlan("proj", "galar", "another thing", ""); err != nil {
		t.Fatalf("a planner with no open PR should take an assignment: %v", err)
	}
	if got, _ := ps.GetState("galar"); got.Phase != "planning" {
		t.Errorf("phase = %q, want planning", got.Phase)
	}
}

// TestAssignPlanRejectsNonPlanners: only a planner has the standing branch and the brief this
// depends on, so the refusal names the role rather than failing obscurely later.
func TestAssignPlanRejectsNonPlanners(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	e := newEngine(st, &stubDeps{root: root})
	if err := e.AssignPlan("proj", "eitri", "x", ""); err == nil || !strings.Contains(err.Error(), "worker") {
		t.Errorf("a worker must be refused, naming its role: %v", err)
	}
	if err := e.AssignPlan("proj", "nobody", "x", ""); err == nil {
		t.Error("an unknown agent must be refused")
	}
	// An empty goal is refused too: the brief would name nothing to plan.
	if err := e.AssignPlan("proj", "eitri", "   ", ""); err == nil {
		t.Error("an empty goal must be refused")
	}
}
