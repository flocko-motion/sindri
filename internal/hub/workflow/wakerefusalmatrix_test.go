package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// wakeRefusalCase is one point in the matrix sd-72af71 asks for: WakeRefusal must agree with
// directive() on whether a push would only be answered with a refusal, over (role, retired,
// clear-armed, escalated, phase, container state, task, awaiting-PR).
type wakeRefusalCase struct {
	name       string
	role       string
	retired    bool
	clearArmed bool
	escalated  bool
	phase      string
	container  string // "" | "active" | "landed"
	task       string
	awaitingPR string // "" | "open" | "rejected"
}

func (c wakeRefusalCase) build(t *testing.T) *Engine {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	a := store.Agent{Name: "ag", Role: c.role, Retired: c.retired, ClearArmed: c.clearArmed}
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	state := store.AgentState{Agent: "ag", Phase: c.phase, Task: c.task}
	if c.container != "" {
		if err := ps.UpsertTask(store.Task{ID: "td-EPIC", Title: "feature", Status: "open"}); err != nil {
			t.Fatal(err)
		}
		state.Container, state.Branch = "td-EPIC", "td-EPIC"
		if c.container == "landed" {
			if err := ps.PutPR(store.PR{ID: "pr-epic", Task: "td-EPIC", Agent: "ag", Branch: "td-EPIC",
				Base: "main", Status: "merged", Kind: "final"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := ps.SetState(state); err != nil {
		t.Fatal(err)
	}
	if c.escalated {
		if err := ps.SetEscalation("ag", "one column or two?"); err != nil {
			t.Fatal(err)
		}
	}
	if c.awaitingPR != "" {
		if err := ps.PutPR(store.PR{ID: "pr-own", Task: "td-own", Agent: "ag", Branch: "td-own",
			Base: "main", Status: c.awaitingPR}); err != nil {
			t.Fatal(err)
		}
	}
	return New(st, &stubDeps{root: t.TempDir()})
}

// isRefusalDirective reports whether dir is one of the three answers a push must never wake an agent
// into hearing: DirRetired, DirEscalated, or DirClearPending.
func isRefusalDirective(dir string) bool {
	return dir == DirRetired || dir == DirClearPending || strings.Contains(dir, "ESCALATED")
}

// TestWakeRefusalAgreesWithDirectiveAcrossTheMatrix asserts the invariant sd-72af71 asks for directly,
// rather than the individual cases that let two rounds of drift through: WakeRefusal(...) != "" iff
// directive(...) answers with a refusal. WakeRefusal is read FIRST in every case — directive() mutates
// the state row (rejection replays, feature-landed resets), so reading it after would see a different
// agent than the one WakeRefusal just answered for.
func TestWakeRefusalAgreesWithDirectiveAcrossTheMatrix(t *testing.T) {
	shapes := []struct {
		name      string
		phase     string
		container string
		task      string
	}{
		{"idle, nothing held", "idle", "", ""},
		{"working a plain task", "working", "", "td-1"},
		{"submitted a plain task", "submitted", "", "td-1"},
		{"gating a plain task", "gating", "", "td-1"},
		{"between subtasks, feature active", "idle", "active", ""},
		{"working a subtask, feature active", "working", "active", "td-1"},
		{"submitted a subtask, feature active", "submitted", "active", "td-1"},
		{"gating a subtask, feature active", "gating", "active", "td-1"},
		{"stale working phase, feature landed", "working", "landed", "td-1"},
		{"idle, feature landed", "idle", "landed", ""},
	}
	flags := []struct {
		name       string
		retired    bool
		clearArmed bool
		escalated  bool
	}{
		{"neither", false, false, false},
		{"retired", true, false, false},
		{"clear-armed", false, true, false},
		{"escalated", false, false, true},
	}
	prStates := []string{"", "open", "rejected"}

	for _, shape := range shapes {
		for _, flag := range flags {
			for _, pr := range prStates {
				if pr != "" && shape.container != "" {
					continue // AwaitingPR is the plain-worker idle branch's own check, not the container one
				}
				c := wakeRefusalCase{
					name: shape.name + "/" + flag.name + "/PR=" + pr, role: "worker",
					retired: flag.retired, clearArmed: flag.clearArmed, escalated: flag.escalated,
					phase: shape.phase, container: shape.container, task: shape.task, awaitingPR: pr,
				}
				t.Run(c.name, func(t *testing.T) {
					e := c.build(t)
					refusal := e.WakeRefusal("repo", "ag")
					dir, err := e.directive(context.Background(), "repo", "ag")
					if err != nil {
						t.Fatal(err)
					}
					if got, want := refusal != "", isRefusalDirective(dir); got != want {
						t.Errorf("WakeRefusal=%q (refuses=%v), directive=%q (refuses=%v) — must agree",
							refusal, got, dir, want)
					}
				})
			}
		}
	}
}

// TestWakeRefusalAgreesWithDirectiveForOtherRoles covers planner and coauthor, which directive()
// never refuses on retired or clear-armed either — reviewer has its own claiming side effects and
// is covered separately (-> TestReviewDirectiveRefusesARetiredReviewer and friends).
func TestWakeRefusalAgreesWithDirectiveForOtherRoles(t *testing.T) {
	for _, role := range []string{"planner", "coauthor"} {
		for _, flag := range []struct {
			name       string
			retired    bool
			clearArmed bool
		}{
			{"retired", true, false},
			{"clear-armed", false, true},
		} {
			t.Run(role+"/"+flag.name, func(t *testing.T) {
				c := wakeRefusalCase{role: role, retired: flag.retired, clearArmed: flag.clearArmed, phase: "idle"}
				e := c.build(t)
				refusal := e.WakeRefusal("repo", "ag")
				dir, err := e.directive(context.Background(), "repo", "ag")
				if err != nil {
					t.Fatal(err)
				}
				if got, want := refusal != "", isRefusalDirective(dir); got != want {
					t.Errorf("WakeRefusal=%q (refuses=%v), directive=%q (refuses=%v) — must agree",
						refusal, got, dir, want)
				}
			})
		}
	}
}
