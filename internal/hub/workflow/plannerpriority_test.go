package workflow

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// claimable answers the question the whole invariant is about, the way the claim queries do: a task
// is claimable when it carries a priority AND the approval gate is not holding it.
func claimable(t *testing.T, ps *store.ProjectStore, id string) bool {
	t.Helper()
	task, ok, err := ps.GetTask(id)
	if err != nil || !ok {
		t.Fatalf("read %s: ok=%v err=%v", id, ok, err)
	}
	appr, _ := ps.GetApproval(id)
	return task.Priority != "" && (appr == "" || appr == "approved")
}

// TestNoPlannerRatingChangesClaimability is THE invariant, asserted against claimability itself
// rather than against the predicate that implements it. Every rating a planner is permitted to make
// leaves the answer to "can a worker take this?" exactly as it was; every rating that would change
// it is refused. A test on the predicate alone would pass even if the verb consulted it wrongly.
func TestNoPlannerRatingChangesClaimability(t *testing.T) {
	for _, tc := range []struct {
		name              string
		approval, current string
		next              string
		wantAllowed       bool
	}{
		{"pending and unrated: proposing a sequence", "pending", "", "high", true},
		{"pending and rated: re-ordering proposals", "pending", "P2", "critical", true},
		{"rejected and unrated", "rejected", "", "high", true},
		{"approved and rated: re-sequencing live backlog", "approved", "P2", "critical", true},
		{"ungated and rated: the same, for a task nobody proposed", "", "P2", "low", true},
		{"approved and unrated: this would release it", "approved", "", "high", false},
		{"ungated and unrated: the same, without an approval row", "", "", "high", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, c, ps := plannerEngine(t, "sd-1", tc.approval)
			if tc.current != "" {
				// The cache row is what every reader sees; the override table is the hub's write
				// path, folded into it on refresh (-> refreshCachedTask).
				if err := ps.UpsertTask(store.Task{ID: "sd-1", Title: "flat proposal", Status: "open", Priority: tc.current}); err != nil {
					t.Fatal(err)
				}
			}
			before := claimable(t, ps, "sd-1")

			var out bytes.Buffer
			code, err := e.CmdPrioritiseTask(c, []string{"sd-1", tc.next}, &out)
			if err != nil {
				t.Fatalf("CmdPrioritiseTask: %v", err)
			}
			if allowed := code == 0; allowed != tc.wantAllowed {
				t.Fatalf("allowed=%v want %v: %s", allowed, tc.wantAllowed, out.String())
			}
			// The invariant, whichever way the call went.
			if after := claimable(t, ps, "sd-1"); after != before {
				t.Errorf("a planner action changed claimability: %v → %v\n%s", before, after, out.String())
			}
			if !tc.wantAllowed {
				// A refusal has to say why, or the planner cannot tell a rule from a fault.
				if !strings.Contains(out.String(), "release") && !strings.Contains(out.String(), "withdraw") {
					t.Errorf("the refusal does not give its reason: %s", out.String())
				}
				// And it must not have written anything.
				task, _, _ := ps.GetTask("sd-1")
				if task.Priority != tc.current {
					t.Errorf("a refused rating still wrote: %q → %q", tc.current, task.Priority)
				}
			}
		})
	}
}

// TestAllowedRatingActuallyLands: the invariant must not be satisfied by refusing everything. Where
// a planner is permitted to re-order, the order really changes.
func TestAllowedRatingActuallyLands(t *testing.T) {
	e, c, ps := plannerEngine(t, "sd-1", "approved")
	if err := ps.UpsertTask(store.Task{ID: "sd-1", Title: "flat proposal", Status: "open", Priority: "P3"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code, err := e.CmdPrioritiseTask(c, []string{"sd-1", "critical"}, &out); err != nil || code != 0 {
		t.Fatalf("re-sequencing an approved task should be allowed: code=%d err=%v %s", code, err, out.String())
	}
	task, _, _ := ps.GetTask("sd-1")
	if task.Priority != "P0" {
		t.Errorf("the new order was not written: %q", task.Priority)
	}
}

// TestTheReplySaysWhetherAnythingWasReleased: a planner that rates a proposal and reads "sd-1 is now
// critical" could reasonably think it had started the work. It has not, and the reply says so.
func TestTheReplySaysWhetherAnythingWasReleased(t *testing.T) {
	e, c, ps := plannerEngine(t, "sd-1", "pending")
	var out bytes.Buffer
	if _, err := e.CmdPrioritiseTask(c, []string{"sd-1", "high"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "awaiting the user's approval") {
		t.Errorf("rating a pending task should say it is still not claimable: %s", out.String())
	}
	// And on an approved one, that the order changed rather than the authorisation.
	if err := ps.UpsertTask(store.Task{ID: "sd-1", Title: "flat proposal", Status: "open", Priority: "P2"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetApproval("sd-1", "approved", ""); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if _, err := e.CmdPrioritiseTask(c, []string{"sd-1", "low"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "order") {
		t.Errorf("re-sequencing should say the order changed: %s", out.String())
	}
}

// TestUnknownPriorityIsRefused: guessing at a near-miss silently re-orders a backlog, and the words
// the hub accepts are the same ones every other surface offers.
func TestUnknownPriorityIsRefused(t *testing.T) {
	e, c, _ := plannerEngine(t, "sd-1", "pending")
	var out bytes.Buffer
	code, err := e.CmdPrioritiseTask(c, []string{"sd-1", "urgentish"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 {
		t.Error("an unknown priority should be refused")
	}
	if !strings.Contains(out.String(), "critical") {
		t.Errorf("the refusal should name the words it accepts: %s", out.String())
	}
}

// TestPrioritiseUsageAndUnknownTask: a malformed call answers on the agent's own stream rather than
// as an internal error it cannot act on.
func TestPrioritiseUsageAndUnknownTask(t *testing.T) {
	e, c, _ := plannerEngine(t, "sd-1", "pending")
	for _, args := range [][]string{{}, {"sd-1"}, {"sd-1", "high", "extra"}} {
		var out bytes.Buffer
		code, err := e.CmdPrioritiseTask(c, args, &out)
		if err != nil || code != 2 {
			t.Errorf("args %v: want usage (exit 2), got code=%d err=%v", args, code, err)
		}
		if !strings.Contains(out.String(), "usage: prioritise-task") {
			t.Errorf("args %v: expected usage, got %s", args, out.String())
		}
	}
	var out bytes.Buffer
	if code, _ := e.CmdPrioritiseTask(c, []string{"sd-nope", "high"}, &out); code == 0 {
		t.Error("an unknown task should be refused")
	}
}

// TestProposalWithAPriorityIsNeverClaimable is the create-task half, and the reason the write order
// matters: a task carrying a rating with no approval row yet IS claimable, so writing the priority
// with the task would open a window in which a worker could take work the user has never seen.
func TestProposalWithAPriorityIsNeverClaimable(t *testing.T) {
	e, c, ps := plannerEngine(t, "sd-seed", "pending")
	var out bytes.Buffer
	if code, err := e.CmdCreateTask(c, []string{"--priority", "critical", "urgent", "thing"}, &out); err != nil || code != 0 {
		t.Fatalf("create-task with a priority: code=%d err=%v %s", code, err, out.String())
	}
	all, err := ps.AllTasks()
	if err != nil {
		t.Fatal(err)
	}
	var created string
	for _, task := range all {
		if task.ID != "sd-seed" {
			created = task.ID
		}
	}
	if created == "" {
		t.Fatal("the task was not created")
	}
	if got, _ := ps.GetApproval(created); got != "pending" {
		t.Errorf("a proposal must be pending, got %q", got)
	}
	task, _, _ := ps.GetTask(created)
	if task.Priority != "P0" {
		t.Errorf("the proposed priority was not recorded: %q", task.Priority)
	}
	if claimable(t, ps, created) {
		t.Error("a rated proposal must not be claimable — approval is what releases it")
	}
}
