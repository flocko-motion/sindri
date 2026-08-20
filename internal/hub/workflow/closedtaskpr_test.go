package workflow

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestClosingATaskRejectsItsPendingPR: a PR exists to land work into a task, so with the task closed
// there is nowhere for it to go and every verdict after that is spent on nothing. pr-sd-c9e829 drew
// two full reviews and a rejection asking for real work, all of it after its task had closed.
func TestClosingATaskRejectsItsPendingPR(t *testing.T) {
	e, ps, id := ownedEngine(t, "open")
	if err := ps.PutPR(store.PR{ID: "pr-" + id, Task: id, Agent: "durin", Branch: id, Status: "open"}); err != nil {
		t.Fatal(err)
	}

	if err := e.CloseTask("proj", id); err != nil {
		t.Fatal(err)
	}

	pr, ok, err := ps.GetPR("pr-" + id)
	if err != nil || !ok {
		t.Fatalf("GetPR: ok=%v err=%v", ok, err)
	}
	if pr.Status != "rejected" {
		t.Fatalf("PR status = %q, want rejected — its task is closed, so nothing can land", pr.Status)
	}
	if !strings.Contains(pr.Feedback, id) {
		t.Errorf("feedback %q never names the closed task, so the author cannot tell why", pr.Feedback)
	}
}

// TestSubmitRefusedWhenItsTaskIsClosed: the refusal comes before the gate, which is the expensive
// part. oin's checkpoint closed its task 126 seconds before its own submit landed, and the PR that
// resulted cost a build, a test suite and two reviewers their time for work with nowhere to go.
func TestSubmitRefusedWhenItsTaskIsClosed(t *testing.T) {
	e, ps, _, caller := submitEngine(t)
	if err := ps.UpsertTask(store.Task{ID: "sd-1", Status: "closed"}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	code, err := e.CmdSubmit(caller, []string{"the work"}, &out)
	if err != nil {
		t.Fatalf("CmdSubmit: %v", err)
	}
	if code == 0 {
		t.Fatalf("submit succeeded against a closed task; said %q", out.String())
	}
	if !strings.Contains(out.String(), "sd-1") {
		t.Errorf("refusal %q never names the task, so the agent cannot tell what closed", out.String())
	}
	if _, ok, _ := ps.GetPR("pr-sd-1"); ok {
		t.Error("a PR was opened against a closed task")
	}
}

// TestAScrappedTasksPRIsLeftToScrapPR: the scrap path discards the branch as well (-> ScrapPR), and
// a rejection written here first would mark the PR terminal and leave that branch standing.
func TestAScrappedTasksPRIsLeftToScrapPR(t *testing.T) {
	e, ps, id := ownedEngine(t, "open")
	if err := ps.PutPR(store.PR{ID: "pr-" + id, Task: id, Agent: "durin", Branch: id, Status: "open"}); err != nil {
		t.Fatal(err)
	}

	if err := e.ScrapTask("proj", id, false, false); err != nil {
		t.Fatal(err)
	}

	pr, ok, err := ps.GetPR("pr-" + id)
	if err != nil || !ok {
		t.Fatalf("GetPR: ok=%v err=%v", ok, err)
	}
	if pr.Status == "rejected" {
		t.Fatal("the scrap path rejected the PR itself, so ScrapPR will skip it and its branch survives")
	}
}
