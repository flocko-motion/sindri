package hub

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/container"
	hubagent "github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// clearableRuntime fakes the tmux/podman runtime, just enough for a clear's calls to succeed with no
// real pod. pane is what a terminal would have DRAWN of the -l literals, since inject reads its own
// text back off it. onSend, if set, is handed each literal as it is typed — a clear blocks until the
// session is observed to have emptied, so a test's only way in is while the call is running.
type clearableRuntime struct {
	container.Runtime
	mu     sync.Mutex
	sent   []string
	pane   string
	onSend func(text string)
}

// paneCols is this fixture's terminal width; the needle is derived from it, so it is a value here
// rather than an assumption baked into the drawing.
const paneCols = 76

// drawn is what a terminal does to typed text — rows wrapped at the box interior and padded out
// behind full-width chrome. Echoing the raw string instead would match needles no pane could.
func drawn(width int, text string) string {
	inner := max(1, width-4)
	var b strings.Builder
	for line := range strings.SplitSeq(text, "\n") {
		for {
			row := line
			if len([]rune(row)) > inner {
				row = string([]rune(row)[:inner])
			}
			b.WriteString("│ " + row + strings.Repeat(" ", inner-len([]rune(row))) + " │\n")
			line = line[len(row):]
			if line == "" {
				break
			}
		}
	}
	return b.String()
}

func (r *clearableRuntime) Running(string) bool                         { return true }
func (r *clearableRuntime) RunningContext(context.Context, string) bool { return true }
func (r *clearableRuntime) Exec(name string, args ...string) ([]byte, error) {
	return r.ExecContext(context.Background(), name, args...)
}
func (r *clearableRuntime) ExecContext(_ context.Context, _ string, args ...string) ([]byte, error) {
	for _, a := range args {
		if a == "capture-pane" {
			r.mu.Lock()
			defer r.mu.Unlock()
			return []byte("\n> \n" + r.pane), nil
		}
	}
	r.mu.Lock()
	r.sent = append(r.sent, strings.Join(args, " "))
	// send-keys -l's literal text is the last arg, after "--"; the fake terminal draws it, so inject's
	// own read-back finds what it just sent.
	var typed string
	if n := len(args); n >= 2 && args[n-2] == "--" {
		typed = args[n-1]
		r.pane += drawn(paneCols, typed)
	}
	hook := r.onSend
	r.mu.Unlock()
	if typed != "" && hook != nil {
		hook(typed)
	}
	return nil, nil
}
func (r *clearableRuntime) Logs(string, int) string { return "" }
func (r *clearableRuntime) Check(io.Writer) error   { return nil }

// joined is every command sent so far, one string, safe against a concurrent send.
func (r *clearableRuntime) joined() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.sent, " | ")
}

// fakeAgent is a coding-agent port whose reported context size the test controls — the adapter a
// hub test supplies at the composition root, isolating the memo under test.
//
// The reading is behind a MUTEX because it crosses goroutines: awaitCleared polls it while the fake
// runtime writes the drop that ends the wait, which is the whole shape under test.
type fakeAgent struct {
	mu     *sync.Mutex
	tokens *int
}

// newFakeAgent is the only way to build one, so the mutex can never be missing — a fake with a
// reading and no lock panics the moment the kickoff goroutine reads it.
func newFakeAgent(tokens int) fakeAgent {
	return fakeAgent{mu: &sync.Mutex{}, tokens: &tokens}
}

// setTokens is how a test simulates the clear landing — the only writer, paired with the read below.
func (f fakeAgent) setTokens(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	*f.tokens = n
}

func (f fakeAgent) DetectState(string) agentport.State { return agentport.Unknown }
func (f fakeAgent) PrepareHome(agentport.HomeSpec) (agentport.Home, error) {
	return agentport.Home{}, nil
}
func (f fakeAgent) RestageCredentials(string) (bool, error) { return false, nil }
func (f fakeAgent) ShortModel(model string) string          { return model }
func (f fakeAgent) HostTokenExpiry() (int64, bool)          { return 0, false }
func (f fakeAgent) ContextUsage(string) (int, int, string, bool) {
	if f.tokens == nil {
		return 0, 0, "", false // the unwired state this package's other tests expect
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return *f.tokens, 1_000_000, "claude-opus-5", true
}
func (f fakeAgent) CompactionThreshold(int) int        { return 1 << 30 }     // never due; not this test's concern
func (f fakeAgent) ModelWindow(string) (int, bool)     { return 0, false }    // not this test's concern
func (f fakeAgent) ModelForTier(string) (string, bool) { return "", false }   // not this test's concern
func (f fakeAgent) ModelMatches(want, got string) bool { return want == got } // not this test's concern
func (f fakeAgent) ToolRunning(string) bool            { return false }       // not this test's concern

// fullAgentWithWorkWaiting seeds an idle, over-threshold worker with a task waiting. The returned
// pointer is the reported context size: set it to simulate a clear.
func fullAgentWithWorkWaiting(t *testing.T) (*Hub, string, fakeAgent) {
	t.Helper()
	fake := newFakeAgent(900_000)
	agentport.Use(fake)
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
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	// The measurement memo is package-global and keyed on project/agent, so a previous test can
	// leave a reading for the same pair — the very staleness under test, arriving by another route.
	h.agents.ForgetContext(testProject, "dvalin")
	return h, "dvalin", fake
}

// TestAFullWorkersOwnAskFiresAClearEndToEnd is the automatic clear-context flow, end to end. The
// clear now blocks until the session is seen to empty, so the directive behind it is already sent by
// the time the ask answers — there is nothing detached left to poll for.
func TestAFullWorkersOwnAskFiresAClearEndToEnd(t *testing.T) {
	h, agent, fake := fullAgentWithWorkWaiting(t)
	w := stillWatchdog(t, h)
	row := store.Agent{Project: testProject, Name: agent}
	w.record(row, true, 0, hubagent.Observation{Runtime: "idle", Digest: "d1"})
	// Fullness is read off the observer's standing sample now, not a live transcript read, so the
	// sweep's own reading is what has to say the agent is full (-> hub/observe.Observation.Fill).
	before, window, _, _ := fake.ContextUsage("")
	w.recordFill(row, fill{tokens: before, window: window})
	rt := &clearableRuntime{}
	// The clear taking effect, at the one moment it can: the call waits for this reading to fall, so
	// a session that answered only after the call returned would be a session that never answered.
	rt.onSend = func(text string) {
		if strings.TrimSpace(text) == "/clear" {
			fake.setTokens(1_000)
		}
	}
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

	sent := rt.joined()
	if !strings.Contains(sent, "/clear") {
		t.Errorf("the session was never sent /clear: %s", sent)
	}
	if !strings.Contains(sent, "sd-1") {
		t.Errorf("the claimed task's own directive should follow the clear, not a generic kickoff: %s", sent)
	}
	if strings.Contains(sent, "Escape") {
		t.Errorf("the automatic clear interrupted the session it was answering: %s", sent)
	}
}

// TestTheBoardReportsAFreshFillAfterAClear: ContextTokens must stop reporting the pre-clear figure
// once a clear lands — there is no status word for fullness anymore to assert on instead.
func TestTheBoardReportsAFreshFillAfterAClear(t *testing.T) {
	h, name, fake := fullAgentWithWorkWaiting(t)
	w := stillWatchdog(t, h)
	row := store.Agent{Project: testProject, Name: name}
	w.record(row, true, 0, hubagent.Observation{Runtime: "idle", Digest: "d1"})
	before, _, _, _ := fake.ContextUsage("")
	w.recordFill(row, fill{tokens: before, window: 1_000_000})
	if view := onlyAgent(t, h); view.ContextTokens != before {
		t.Fatalf("precondition: the board should report the pre-clear fill, got %d", view.ContextTokens)
	}

	fake.setTokens(1_000) // the clear happens: the session's context is gone
	h.agents.ForgetContext(testProject, name)

	if view := onlyAgent(t, h); view.ContextTokens != 0 {
		t.Errorf("the board still reports the pre-clear fill of %d tokens", view.ContextTokens)
	}
}
