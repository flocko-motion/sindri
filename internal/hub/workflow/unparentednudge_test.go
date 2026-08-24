// package: hub/workflow / unparentednudge_test
// type:    logic (tests for create-task's unparented-sibling nudge)
// job:     covers unparentedNudge — named siblings, dropping a reparented one, and staying
// silent when the new proposal itself has a parent.
// limits:  this behaviour only; the rest of create-task/edit-task is planner_test.go's.
package workflow

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// idByTitle finds a task's minted id by its title, so a test can check for it as a plain value
// rather than scraping it back out of CmdCreateTask's own reply text.
func idByTitle(t *testing.T, ps *store.ProjectStore, title string) string {
	t.Helper()
	tasks, err := ps.AllTasks()
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	for _, tk := range tasks {
		if tk.Title == title {
			return tk.ID
		}
	}
	t.Fatalf("no task titled %q", title)
	return ""
}

// TestUnparentedNudgeNamesRecentSiblings (sd-ac9800): a planner filing a task with no --parent,
// having already filed others without one, must hear about them right in the reply — the moment
// it still holds the context to hang them all under a container it proposes next.
func TestUnparentedNudgeNamesRecentSiblings(t *testing.T) {
	e, c, ps := plannerEngine(t, "td-seed", "")

	var out bytes.Buffer
	if _, err := e.CmdCreateTask(c, []string{"first", "flat", "task"}, &out); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if strings.Contains(out.String(), "also proposed") {
		t.Errorf("the FIRST flat proposal has no sibling to name yet: %s", out.String())
	}
	firstID := idByTitle(t, ps, "first flat task")

	out.Reset()
	if _, err := e.CmdCreateTask(c, []string{"second", "flat", "task"}, &out); err != nil {
		t.Fatalf("create second: %v", err)
	}
	if !strings.Contains(out.String(), firstID) {
		t.Errorf("a second flat proposal should nudge about the first: %s", out.String())
	}
}

// TestUnparentedNudgeDropsATaskOnceItGetsAParent (sd-ac9800): reparenting an existing proposal is
// the planner doing the right thing, and it must never be logged as another flat proposal or keep
// being named once it has a home — a reminder firing while the planner tidies is exactly backwards.
func TestUnparentedNudgeDropsATaskOnceItGetsAParent(t *testing.T) {
	e, c, ps := plannerEngine(t, "td-container", "")

	var out bytes.Buffer
	if _, err := e.CmdCreateTask(c, []string{"first", "flat", "task"}, &out); err != nil {
		t.Fatalf("create first: %v", err)
	}
	firstID := idByTitle(t, ps, "first flat task")

	out.Reset()
	if _, err := e.CmdCreateTask(c, []string{"second", "flat", "task"}, &out); err != nil {
		t.Fatalf("create second: %v", err)
	}
	if !strings.Contains(out.String(), firstID) {
		t.Fatalf("precondition: second proposal should nudge about the first: %s", out.String())
	}
	secondID := idByTitle(t, ps, "second flat task")

	out.Reset()
	if _, err := e.CmdEditTask(c, []string{firstID, "--parent", "td-container"}, &out); err != nil {
		t.Fatalf("reparent: %v", err)
	}

	out.Reset()
	if _, err := e.CmdCreateTask(c, []string{"third", "flat", "task"}, &out); err != nil {
		t.Fatalf("create third: %v", err)
	}
	if strings.Contains(out.String(), firstID) {
		t.Errorf("a reparented task must not be named again: %s", out.String())
	}
	if !strings.Contains(out.String(), secondID) {
		t.Errorf("the still-unparented second proposal should still be named: %s", out.String())
	}
}

// TestNoNudgeWhenTheNewProposalItselfHasAParent (sd-ac9800): the nudge is about the planner ALSO
// filing flat right now — a proposal that itself names a container has nothing to be nudged about,
// however many unparented siblings came before it.
func TestNoNudgeWhenTheNewProposalItselfHasAParent(t *testing.T) {
	e, c, _ := plannerEngine(t, "td-container", "")

	var out bytes.Buffer
	if _, err := e.CmdCreateTask(c, []string{"first", "flat", "task"}, &out); err != nil {
		t.Fatalf("create first: %v", err)
	}

	out.Reset()
	if _, err := e.CmdCreateTask(c, []string{"--parent", "td-container", "second", "task"}, &out); err != nil {
		t.Fatalf("create second: %v", err)
	}
	if strings.Contains(out.String(), "also proposed") {
		t.Errorf("a proposal that itself has a parent should not be nudged: %s", out.String())
	}
}
