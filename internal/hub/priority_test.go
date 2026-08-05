package hub

import (
	"os/exec"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestGitHubTaskPriorityStaysHubSide: re-rating a gh-* task records a hub-side
// priority override (like os-*), never routing through td or GitHub — a gh-* task's
// only outbound GitHub write is close-on-merge. Needs a td store for SyncTasks.
func TestGitHubTaskPriorityStaysHubSide(t *testing.T) {
	if _, err := exec.LookPath("td"); err != nil {
		t.Skip("td CLI not installed")
	}
	h := newHub(t)
	root := t.TempDir()
	if out, err := exec.Command("td", "-w", root, "init").CombinedOutput(); err != nil {
		t.Fatalf("td init: %s", out)
	}
	h.repo(root) // register so projectRoot resolves
	tag := RepoTag(root)

	if err := h.wf.SetPriority(tag, "gh-9", "P1"); err != nil {
		t.Fatalf("SetPriority on a gh-* task: %v", err)
	}
	ov, err := h.store.For(tag).PriorityOverrides()
	if err != nil {
		t.Fatal(err)
	}
	if ov["gh-9"] != "P1" {
		t.Fatalf("gh-9 priority should be a hub-side override P1, got %q", ov["gh-9"])
	}

	// EditTask with a priority takes the same hub-side path.
	if err := h.wf.EditTask(tag, "gh-9", api.TaskSpec{Priority: "P2"}); err != nil {
		t.Fatalf("EditTask on a gh-* task: %v", err)
	}
	ov, _ = h.store.For(tag).PriorityOverrides()
	if ov["gh-9"] != "P2" {
		t.Fatalf("gh-9 priority override after EditTask should be P2, got %q", ov["gh-9"])
	}
}
