package hub

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/container"
	hubagent "github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// clearableRuntime fakes the tmux/podman runtime, just enough for FireClear's calls to succeed with
// no real pod. sent is mutex-guarded: FireClear's own kickoff goroutine writes it too.
type clearableRuntime struct {
	container.Runtime
	mu   sync.Mutex
	sent []string
}

func (r *clearableRuntime) Running(string) bool                         { return true }
func (r *clearableRuntime) RunningContext(context.Context, string) bool { return true }
func (r *clearableRuntime) Exec(name string, args ...string) ([]byte, error) {
	return r.ExecContext(context.Background(), name, args...)
}
func (r *clearableRuntime) ExecContext(_ context.Context, _ string, args ...string) ([]byte, error) {
	for _, a := range args {
		if a == "capture-pane" {
			return []byte("\n> \n"), nil
		}
	}
	r.mu.Lock()
	r.sent = append(r.sent, strings.Join(args, " "))
	r.mu.Unlock()
	return nil, nil
}
func (r *clearableRuntime) Logs(string, int) string { return "" }
func (r *clearableRuntime) Check(io.Writer) error   { return nil }

// joined is every command sent so far, one string, safe against the kickoff goroutine's own writes.
func (r *clearableRuntime) joined() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.sent, " | ")
}

// fakeAgent is a coding-agent port whose reported context size the test controls — the adapter a
// hub test supplies at the composition root, isolating the memo under test.
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
func (f fakeAgent) CompactionThreshold(int) int        { return 1 << 30 }     // never due; not this test's concern
func (f fakeAgent) ModelWindow(string) (int, bool)     { return 0, false }    // not this test's concern
func (f fakeAgent) ModelForTier(string) (string, bool) { return "", false }   // not this test's concern
func (f fakeAgent) ModelMatches(want, got string) bool { return want == got } // not this test's concern
func (f fakeAgent) ToolRunning(string) bool            { return false }       // not this test's concern

// fullAgentWithWorkWaiting seeds an idle, over-threshold worker with a task waiting. The returned
// pointer is the reported context size: set it to simulate a clear.
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

// TestAFullWorkersOwnAskFiresAClearEndToEnd is the automatic clear-context flow, end to end. Checked
// on the session, not a second AgentDirective call: the claim's own hand-over lands behind /clear.
func TestAFullWorkersOwnAskFiresAClearEndToEnd(t *testing.T) {
	h, agent, tokens := fullAgentWithWorkWaiting(t)
	w := stillWatchdog(t, h)
	w.record(store.Agent{Project: testProject, Name: agent}, true, 0, hubagent.Observation{Runtime: "idle", Digest: "d1"})
	rt := &clearableRuntime{}
	container.Use(rt)
	t.Cleanup(container.UseDefault)

	dir, err := h.wf.AgentDirective(context.Background(), testProject, agent)
	if err != nil {
		t.Fatal(err)
	}
	if dir != workflow.DirPreparing {
		t.Fatalf("a full agent's claim should fire a clear and answer %q, got %q", workflow.DirPreparing, dir)
	}
	if st, _ := h.store.For(testProject).GetState(agent); st.Task != "sd-1" {
		t.Errorf("state.Task = %q, want sd-1 — the claim holds regardless of the clear firing", st.Task)
	}

	*tokens = 1_000 // the clear happens: the session's context is gone
	h.agents.ForgetContext(testProject, agent)

	// FireClear's kickoff fires clearKickoffDelay later in its own goroutine — poll rather than
	// sleep a fixed margin over that delay, so this can't flake under a loaded gate.
	var sent string
	deadline := time.Now().Add(10 * time.Second)
	for {
		sent = rt.joined()
		if strings.Contains(sent, "sd-1") || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !strings.Contains(sent, "/clear") {
		t.Errorf("the session was never sent /clear: %s", sent)
	}
	if !strings.Contains(sent, "sd-1") {
		t.Errorf("the claimed task's own directive should be queued behind the clear, not a generic kickoff: %s", sent)
	}
	if strings.Contains(sent, "Escape") {
		t.Errorf("the automatic clear interrupted the session it was answering: %s", sent)
	}
}

// TestTheBoardReportsAFreshFillAfterAClear: ContextTokens must stop reporting the pre-clear figure
// once a clear lands — there is no status word for fullness anymore to assert on instead.
func TestTheBoardReportsAFreshFillAfterAClear(t *testing.T) {
	h, name, tokens := fullAgentWithWorkWaiting(t)
	w := stillWatchdog(t, h)
	row := store.Agent{Project: testProject, Name: name}
	w.record(row, true, 0, hubagent.Observation{Runtime: "idle", Digest: "d1"})
	w.recordFill(row, fill{tokens: *tokens, window: 1_000_000})
	if view := onlyAgent(t, h); view.ContextTokens != *tokens {
		t.Fatalf("precondition: the board should report the pre-clear fill, got %d", view.ContextTokens)
	}

	*tokens = 1_000 // the clear happens: the session's context is gone
	h.agents.ForgetContext(testProject, name)

	if view := onlyAgent(t, h); view.ContextTokens != 0 {
		t.Errorf("the board still reports the pre-clear fill of %d tokens", view.ContextTokens)
	}
}
