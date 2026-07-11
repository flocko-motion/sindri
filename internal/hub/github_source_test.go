package hub

import (
	"os/exec"
	"testing"
)

// TestPriorityNoneRename: the lowest tier now reads "none" everywhere, but the old
// input words still resolve to P4 so muscle memory and stored input keep working.
func TestPriorityNoneRename(t *testing.T) {
	if got := PriorityLabel("P4"); got != "none" {
		t.Errorf("PriorityLabel(P4) = %q, want none", got)
	}
	for _, w := range []string{"none", "trivial", "minor"} {
		if got := PriorityCode(w); got != "P4" {
			t.Errorf("PriorityCode(%q) = %q, want P4", w, got)
		}
	}
	last := PriorityWords[len(PriorityWords)-1]
	if last != "none" {
		t.Errorf("PriorityWords should end in none, got %q", last)
	}
	for _, w := range PriorityWords {
		if w == "trivial" {
			t.Error("PriorityWords must not advertise the old word trivial")
		}
	}
}

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

	if err := h.SetPriority(tag, "gh-9", "P1"); err != nil {
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
	if err := h.EditTask(tag, "gh-9", TaskSpec{Priority: "P2"}); err != nil {
		t.Fatalf("EditTask on a gh-* task: %v", err)
	}
	ov, _ = h.store.For(tag).PriorityOverrides()
	if ov["gh-9"] != "P2" {
		t.Fatalf("gh-9 priority override after EditTask should be P2, got %q", ov["gh-9"])
	}
}
