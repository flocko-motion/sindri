package container

import (
	"context"
	"testing"
	"time"
)

// countingRuntime is a noop that records how often it was listed and answers with whatever the
// test says exists at that moment.
type countingRuntime struct {
	noop
	calls int
	pods  []string
}

func (r *countingRuntime) ListByLabelContext(context.Context, string, string) ([]string, error) {
	r.calls++
	return r.pods, nil
}

// withRuntime swaps the active runtime for one test and clears the memo either side, so the cache's
// process-wide state cannot leak between tests.
func withRuntime(t *testing.T, r Runtime) {
	t.Helper()
	prev := active
	Use(r)
	resetListMemo()
	t.Cleanup(func() { Use(prev); resetListMemo() })
}

func resetListMemo() {
	listMemo.mu.Lock()
	listMemo.at, listMemo.key, listMemo.pods, listMemo.err = time.Time{}, "", nil, nil
	listMemo.mu.Unlock()
}

// TestFreshListSeesAContainerCreatedAfterTheCache is the bug this exists for. A pod created after
// the memo was filled is absent from the memoized answer, and absence from a listing is what the
// watchdog reads as an agent being gone — so a freshly launched agent was declared down while it
// was demonstrably running.
func TestFreshListSeesAContainerCreatedAfterTheCache(t *testing.T) {
	r := &countingRuntime{pods: []string{"old-pod"}}
	withRuntime(t, r)
	ctx := context.Background()

	if _, err := ListByLabelCached(ctx, "sindri.project", ""); err != nil {
		t.Fatal(err)
	}
	r.pods = append(r.pods, "new-pod") // a launch, moments later

	cached, err := ListByLabelCached(ctx, "sindri.project", "")
	if err != nil {
		t.Fatal(err)
	}
	if contains(cached, "new-pod") {
		t.Fatal("precondition: the memo should still be serving the older answer")
	}

	fresh, err := ListByLabelFresh(ctx, "sindri.project", "")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(fresh, "new-pod") {
		t.Errorf("a fresh list must see the new pod, got %v", fresh)
	}
}

// TestFreshListPrimesTheMemo: the sweep pays for a real listing every watchInterval, and the board
// reads that follow it should be answered from that newer result rather than the one it overtook.
func TestFreshListPrimesTheMemo(t *testing.T) {
	r := &countingRuntime{pods: []string{"old-pod"}}
	withRuntime(t, r)
	ctx := context.Background()

	if _, err := ListByLabelCached(ctx, "sindri.project", ""); err != nil {
		t.Fatal(err)
	}
	r.pods = append(r.pods, "new-pod")
	if _, err := ListByLabelFresh(ctx, "sindri.project", ""); err != nil {
		t.Fatal(err)
	}

	before := r.calls
	cached, err := ListByLabelCached(ctx, "sindri.project", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.calls != before {
		t.Errorf("the read after a fresh list should come from the memo, but it listed again")
	}
	if !contains(cached, "new-pod") {
		t.Errorf("the memo should hold the fresh answer, got %v", cached)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
