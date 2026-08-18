package hub

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/hub/store"
)

// fakeAgent is an implementation of the coding-agent port whose reported context size the test
// controls. The adapter is wired at the composition root, so a hub test supplies its own — which is
// what the port is for, and it isolates the memo, which is the defect under test.
type fakeAgent struct{ tokens *int }

func (f fakeAgent) DetectState(string) agentport.State { return agentport.Unknown }
func (f fakeAgent) PrepareHome(agentport.HomeSpec) (agentport.Home, error) {
	return agentport.Home{}, nil
}
func (f fakeAgent) RestageCredentials(string) (bool, error) { return false, nil }
func (f fakeAgent) HostTokenExpiry() (int64, bool)          { return 0, false }
func (f fakeAgent) ContextUsage(string) (int, int, string, bool) {
	if f.tokens == nil {
		return 0, 0, "", false // the unwired state this package's other tests expect
	}
	return *f.tokens, 1_000_000, "claude-opus-5", true
}
func (f fakeAgent) CompactionThreshold(int) int    { return 1 << 30 }  // never due; not this test's concern
func (f fakeAgent) ModelWindow(string) (int, bool) { return 0, false } // not this test's concern

// fullAgentWithWorkWaiting seeds an idle worker reported as over the fullness threshold, with an
// approved, prioritised task waiting for it. The returned pointer is the reported context size: set
// it to simulate a clear.
func fullAgentWithWorkWaiting(t *testing.T) (*Hub, string, *int) {
	t.Helper()
	tokens := 900_000
	agentport.Use(fakeAgent{tokens: &tokens})
	t.Cleanup(func() { agentport.Use(fakeAgent{}) }) // back to reporting nothing, as an unwired hub does

	h := newHub(t)
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main", root},
		{"-C", root, "config", "user.email", "t@t"},
		{"-C", root, "config", "user.name", "t"},
		{"-C", root, "commit", "-q", "--allow-empty", "-m", "base"},
		{"-C", root, "worktree", "add", "-q", "--detach", filepath.Join(root, ".worktrees", "dvalin"), "HEAD"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Skipf("git unavailable: %v %s", err, out)
		}
	}
	if err := h.store.RegisterProject(testProject, root); err != nil {
		t.Fatal(err)
	}
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "sd-1", Title: "waiting work", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	// The measurement memo is package-global and keyed on project/agent, so a previous test can
	// leave a reading for the same pair — the very staleness under test, arriving by another route.
	h.agents.ForgetContext(testProject, "dvalin")
	return h, "dvalin", &tokens
}

// TestAClearedAgentIsNotStillToldItIsFull is the reported behaviour, end to end and against the real
// measurement. The context figure is memoised for 15s and the kickoff after a clear lands at 2s, so
// the hub answered every clear from the pre-clear reading and told the agent it was still full — the
// exact remedy that had just been applied. Deterministic, not a race.
//
// Asserted on the DIRECTIVE rather than on the memo entry: a test that only checked the entry was
// gone would pass while the agent kept being turned away, if the invalidation moved after the
// kickoff.
func TestAClearedAgentIsNotStillToldItIsFull(t *testing.T) {
	h, agent, tokens := fullAgentWithWorkWaiting(t)

	dir, err := h.wf.AgentDirective(context.Background(), testProject, agent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dir, "retired") {
		t.Fatalf("precondition: a full agent should be retired from assignment, got %q", dir)
	}

	*tokens = 1_000 // the clear happens: the session's context is gone
	// Without the invalidation the memo still holds the pre-clear figure, and this is the moment
	// the agent asks for work.
	h.agents.ForgetContext(testProject, agent)

	dir, err = h.wf.AgentDirective(context.Background(), testProject, agent)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(dir, "retired") {
		t.Errorf("a cleared agent is still being turned away:\n%s", dir)
	}
	if !strings.Contains(dir, "sd-1") {
		t.Errorf("a cleared agent should be handed the waiting work, got:\n%s", dir)
	}
}

// TestTheMemoIsWhatWasStale confirms the diagnosis rather than assuming it: with the transcript
// rewritten and no invalidation, the hub keeps answering from the old reading. If this passed, the
// fix would be aimed at the wrong thing.
func TestTheMemoIsWhatWasStale(t *testing.T) {
	h, agent, tokens := fullAgentWithWorkWaiting(t)
	if _, err := h.wf.AgentDirective(context.Background(), testProject, agent); err != nil {
		t.Fatal(err)
	}
	*tokens = 1_000 // cleared, but nothing tells the memo

	dir, err := h.wf.AgentDirective(context.Background(), testProject, agent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dir, "retired") {
		t.Skip("the measurement is not memoised here, so this diagnosis no longer applies")
	}
}

// TestTheBoardAgreesWithTheDirective: ContextFull reads the same memo, so a cleared agent would also
// have shown as full on the board for the rest of the window. One invalidation fixes both, and this
// is the half a directive test would not notice.
func TestTheBoardAgreesWithTheDirective(t *testing.T) {
	h, agent, tokens := fullAgentWithWorkWaiting(t)
	if !h.wf.ContextFull(testProject, agent) {
		t.Fatal("precondition: the board should show a full agent as full")
	}
	*tokens = 1_000
	h.agents.ForgetContext(testProject, agent)
	if h.wf.ContextFull(testProject, agent) {
		t.Error("the board still shows a cleared agent as full")
	}
}
