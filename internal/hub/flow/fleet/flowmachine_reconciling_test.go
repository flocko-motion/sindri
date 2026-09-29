package fleet

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// TestTheLoopStartsAPodForWorkNobodyAnnounced is the one test that runs the real loop, and it runs
// it DEAF: every announcement suppressed, so nothing but the states' own polls can carry it. The
// claim under test is the engine's own law — "a topic may only SHORTEN latency" — and the way to
// prove a hint is a hint is to drop every one and watch the same thing happen, slower.
//
// The specific thing that failed here was the loop reaching a subject nobody expected it to reach:
// a stopped agent is not asked for by anything, so only the fleet looking at it on its own account
// brings it back for work waiting in a backlog it cannot see.
func TestTheLoopStartsAPodForWorkNobodyAnnounced(t *testing.T) {
	deaf = true
	t.Cleanup(func() { deaf = false })

	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: "bombur", Role: "worker", Workspace: ".worktrees/bombur", Stopped: true}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "sd-1", Title: "waiting work", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	// Its pod is gone, which is the whole point: nothing can be typed at it, so nothing about this
	// can be carried by telling the agent anything.
	deps := &stubDeps{Root: root, Down: true}
	e := newEngine(t, st, deps).Reconciling()
	t.Cleanup(e.Close)

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if len(deps.Launched()) > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	st2, _ := ps.GetState("bombur")
	t.Fatalf("the loop never brought a stopped agent back for work waiting for it; it stands in %q", st2.Phase)
}
