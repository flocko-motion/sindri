package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// ratedModel is the Tasks tab holding one task in the given approval state.
func ratedModel(approval string, extra ...api.Task) model {
	m := newModel(nil, nil, "")
	m.tab, m.scopeRepo = 0, false
	m.w, m.h = 120, 30
	m.state = api.BoardState{Tasks: append([]api.Task{
		{ID: "sd-1", Title: "a task", Status: "open", Approval: approval},
	}, extra...)}
	return m
}

// TestRatingAPendingTaskOffersApproval is the trap this closes. All three claim queries require the
// gate to be clear as well as a priority to be set, so rating a pending task leaves it exactly as
// unclaimable as before — while the row visibly gains its priority, which reads as work released.
func TestRatingAPendingTaskOffersApproval(t *testing.T) {
	m := ratedModel("pending")
	if cmd := m.approveAfterPriority("sd-1"); cmd == nil {
		t.Fatal("rating a pending task should offer the approve that would release it")
	} else if _, ok := cmd().(openApproveAfterPriorityMsg); !ok {
		t.Errorf("the follow-up should open the approve confirm, got %T", cmd())
	}
}

// TestRatingAnApprovedTaskAsksNothing: the offer exists to explain a gate that is still shut. On a
// task already through it there is nothing to explain, and a second prompt would be friction.
func TestRatingAnApprovedTaskAsksNothing(t *testing.T) {
	for _, approval := range []string{"approved", ""} {
		m := ratedModel(approval)
		if cmd := m.approveAfterPriority("sd-1"); cmd != nil {
			t.Errorf("approval=%q: no approve should be offered", approval)
		}
	}
}

// TestDecliningTheApproveChangesNothing: declining must be a real choice, so it issues no command —
// the task keeps the priority just set and stays in the backlog, which the note says it will.
func TestDecliningTheApproveChangesNothing(t *testing.T) {
	m := ratedModel("pending")
	m.openApproveAfterPriorityChoice("sd-1")
	if !m.choice.active {
		t.Fatal("the approve confirm did not open")
	}
	if m.choice.values[0] != "cancel" {
		t.Errorf("the confirm should open on cancel, got %q", m.choice.values[0])
	}
	if cmd := m.choice.apply("cancel"); cmd != nil {
		t.Error("declining must not run anything")
	}
	// The consequence has to be on screen, not implied: which act releases the work, and what
	// declining leaves behind.
	note := m.choice.note
	for _, want := range []string{"releases it", "priority alone does not", "stays in the backlog"} {
		if !strings.Contains(note, want) {
			t.Errorf("the confirm does not say %q:\n%s", want, note)
		}
	}
}

// TestApproveAfterRatingNeverTouchesChildren is the bulk-approval guard. A scoped rating can carry
// to children that are themselves pending; approving those in the same breath would turn one
// confirm into a verdict on work the user rated but never read.
func TestApproveAfterRatingNeverTouchesChildren(t *testing.T) {
	m := ratedModel("pending",
		api.Task{ID: "sd-2", Title: "kid", Status: "open", ParentID: "sd-1", Approval: "pending"},
		api.Task{ID: "sd-3", Title: "kid", Status: "open", ParentID: "sd-1", Approval: "pending"},
	)
	m.openApproveAfterPriorityChoice("sd-1")

	// The count is reported, so the remaining verdicts are visible rather than silently skipped.
	if !strings.Contains(m.choice.note, "2 tasks below") {
		t.Errorf("the pending children should be counted in the note:\n%s", m.choice.note)
	}
	if !strings.Contains(m.choice.note, "does not touch them") {
		t.Errorf("the note should say the children are excluded:\n%s", m.choice.note)
	}
	// And only one task is named as the thing being approved.
	if strings.Contains(strings.Join(m.choice.options, " "), "sd-2") {
		t.Errorf("no option may approve a child: %v", m.choice.options)
	}
}

// TestApproveConfirmOffersOnlyTheOneTask: the option list is the other half of the same guard — a
// "task + subtasks" option here would reintroduce the bulk verdict the note promises not to give.
func TestApproveConfirmOffersOnlyTheOneTask(t *testing.T) {
	m := ratedModel("pending",
		api.Task{ID: "sd-2", Title: "kid", Status: "open", ParentID: "sd-1", Approval: "pending"},
	)
	m.openApproveAfterPriorityChoice("sd-1")
	if len(m.choice.values) != 2 {
		t.Fatalf("expected cancel + approve, got %v", m.choice.values)
	}
	for _, v := range m.choice.values {
		if strings.Contains(v, "tree") || strings.Contains(v, "subtask") {
			t.Errorf("the confirm offers a subtree approve: %v", m.choice.values)
		}
	}
}
