package td

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/task"
)

// seed makes a store holding one started task, driven to status, and returns both.
func seed(t *testing.T, status string) (root, id string) {
	t.Helper()
	root = tdStore(t, [][]string{{"a task title long enough for td to accept"}})
	tasks, err := Tasks(root, task.FilterAll)
	if err != nil || len(tasks) == 0 {
		t.Fatalf("no task after create (err %v)", err)
	}
	id = tasks[0].ID
	if out, err := exec.Command("td", "-w", root, "start", id).CombinedOutput(); err != nil {
		t.Fatalf("td start: %s", out)
	}
	if status == "in_review" {
		if out, err := exec.Command("td", "-w", root, "review", id).CombinedOutput(); err != nil {
			t.Fatalf("td review: %s", out)
		}
	}
	return root, id
}

// status reads the live status from td.
func status(t *testing.T, root, id string) string {
	t.Helper()
	got, err := Get(root, id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return got.Status
}

// TestCloseLeavesReviewWhenTdRefuses: td declines a close while an issue is in review and points at
// `td approve`, which sindri cannot use — it created and started the task, so td counts it involved.
// The close has to get there anyway, or a merged PR leaves its task open.
func TestCloseLeavesReviewWhenTdRefuses(t *testing.T) {
	root, id := seed(t, "in_review")
	if got := status(t, root, id); got != "in_review" {
		t.Fatalf("seed: status %q, want in_review", got)
	}
	if err := Close(root, id, "merged via pr-test"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := status(t, root, id); got != "closed" {
		t.Errorf("status %q after Close, want closed", got)
	}
}

// TestCloseWorksDirectlyWhenNothingResists keeps the ordinary path honest: no review to leave, so
// the first close carries it.
func TestCloseWorksDirectlyWhenNothingResists(t *testing.T) {
	root, id := seed(t, "in_progress")
	if err := Close(root, id, "merged via pr-test"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := status(t, root, id); got != "closed" {
		t.Errorf("status %q after Close, want closed", got)
	}
}

// TestRefusedMutationIsAnError is the silence that hid the whole bug: td prints ERROR and exits 0,
// so a caller reading only the exit code believes a refusal succeeded.
func TestRefusedMutationIsAnError(t *testing.T) {
	root, id := seed(t, "in_review")
	// The bare invocation td refuses, reached through the same runner every mutation uses.
	err := mutate(root, "close", id, "--self-close-exception", "merged via pr-test")
	if err == nil {
		t.Fatalf("a refused close must be an error; td exits 0 and only says so in its output")
	}
	if !strings.Contains(err.Error(), "refused") {
		t.Errorf("the error should name it a refusal: %v", err)
	}
	if got := status(t, root, id); got != "in_review" {
		t.Errorf("the refused close must change nothing, got %q", got)
	}
}

// TestRefusalReadsColourisedOutput: td colourises, so the marker never begins a line.
func TestRefusalReadsColourisedOutput(t *testing.T) {
	if got := refusal("\x1b[38;5;196mERROR: cannot close td-1: issue is in review\x1b[m"); got == "" {
		t.Error("a colourised ERROR line must be recognised")
	} else if !strings.Contains(got, "issue is in review") {
		t.Errorf("refusal message = %q, want td's reason", got)
	}
	if got := refusal("CLOSED td-1 (self-close exception)"); got != "" {
		t.Errorf("ordinary success must not read as a refusal, got %q", got)
	}
}
