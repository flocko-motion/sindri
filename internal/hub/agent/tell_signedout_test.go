package agent

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
)

// signedOutPane is Claude's auth banner as the classifier matches it.
const signedOutPane = "● Login expired · Please run /login"

// idlePane is an ordinary prompt box — an agent that will take what it is told.
const idlePane = "\n> \n"

// fakeRuntime answers for a container backend: what the pane says, what was typed into it, and
// which pods were torn down — the three facts these cases turn on.
type fakeRuntime struct {
	container.Runtime // every method the tests never reach
	pane              string
	sent              []string
	removed           []string
}

func (f *fakeRuntime) Running(string) bool { return true }

// RunningContext answers as Running does: a liveness check that takes a deadline is asking the same
// question, and a fake that answered differently would make "alive" depend on which one a path used.
func (f *fakeRuntime) RunningContext(context.Context, string) bool { return true }

func (f *fakeRuntime) Exec(name string, args ...string) ([]byte, error) {
	return f.ExecContext(context.Background(), name, args...)
}

func (f *fakeRuntime) ExecContext(_ context.Context, _ string, args ...string) ([]byte, error) {
	switch {
	case containsArg(args, "capture-pane"):
		return []byte(f.pane), nil
	case containsArg(args, "send-keys") && containsArg(args, "-l"):
		f.sent = append(f.sent, args[len(args)-1])
	}
	return nil, nil
}

func (f *fakeRuntime) Rm(name string) error {
	f.removed = append(f.removed, name)
	return nil
}

// Check fails, so a Launch this fake reaches stops at the pre-flight instead of building an image.
func (f *fakeRuntime) Check(io.Writer) error { return errNothingToLaunchInto }

var errNothingToLaunchInto = errors.New("fake runtime: nothing to launch into")

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// tellDeps is a minimal agent.Deps for the delivery paths.
type tellDeps struct{}

func (tellDeps) Notify()                                     {}
func (tellDeps) ContainerName(_, name string) string         { return "pod-" + name }
func (tellDeps) ProjectRoot(string) string                   { return "" }
func (tellDeps) ProjectConfig(string) (config.Config, error) { return config.Config{}, nil }
func (tellDeps) ArchitectureDoc(string) string               { return "" }
func (tellDeps) RefreshTask(_, _ string) error               { return nil }
func (tellDeps) Rehydrate(_, _ string)                       {}

// paneReader is a coding-agent backend that reads the one banner these cases turn on. The port
// defaults to a no-op that classifies nothing, and what the real classifier makes of a screen is
// its own package's business (-> adapter/agent/claude); here the question is what the hub does with
// the verdict.
type paneReader struct{ agentport.Agent }

func (paneReader) DetectState(screen string) agentport.State {
	if strings.Contains(strings.ToLower(screen), "please run /login") {
		return agentport.SignedOut
	}
	return agentport.Idle
}

// tellFixture wires a service over a fake backend showing pane, with one agent registered.
func tellFixture(t *testing.T, name, pane string) (*Service, *fakeRuntime) {
	t.Helper()
	_, st := newService(t)
	s := New(st, tellDeps{}, nil)
	if err := st.For("proj").PutAgent(store.Agent{Name: name, Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	f := &fakeRuntime{pane: pane}
	container.Use(f)
	agentport.Use(paneReader{})
	t.Cleanup(func() {
		container.UseDefault()          // nothing running, as an unwired process finds it
		agentport.Use(unreadablePane{}) // back to classifying nothing, as an unwired hub does
	})
	forgetObservations()
	t.Cleanup(forgetObservations)
	return s, f
}

// unreadablePane restores the port to saying nothing about any screen.
type unreadablePane struct{ agentport.Agent }

func (unreadablePane) DetectState(string) agentport.State { return agentport.Unknown }

// forgetObservations clears the pane memo, so one case's screen is never another's answer.
func forgetObservations() {
	runtimeMemo.mu.Lock()
	runtimeMemo.at, runtimeMemo.val = nil, nil
	runtimeMemo.mu.Unlock()
}

// TestAHubMessageStillRefusesASignedOutPane is the half of the guard that must survive: a verdict
// or an assignment has no human behind it to notice that it landed in an input box unread, so it
// fails loudly instead of vanishing — and the refusal names the restart that fixes it.
func TestAHubMessageStillRefusesASignedOutPane(t *testing.T) {
	s, f := tellFixture(t, "eitri", signedOutPane)
	err := s.Inject("proj", "eitri", "the verdict is in")
	if err == nil {
		t.Fatal("a hub-originated message into a signed-out pane must fail rather than vanish")
	}
	if !strings.Contains(err.Error(), "restart") {
		t.Errorf("the refusal must name the remedy, got %q", err)
	}
	if len(f.sent) != 0 {
		t.Errorf("nothing should have been typed, got %q", f.sent)
	}
}

// TestSendAnywayOverrulesThePane: the reading is a look at a screen and can be older than what the
// user knows — they may have renewed the host's token seconds ago. Their answer wins, and if they
// are wrong the failure is visible and costs a message.
func TestSendAnywayOverrulesThePane(t *testing.T) {
	s, f := tellFixture(t, "dvalin", signedOutPane)
	if err := s.Tell("proj", "dvalin", "carry on", "user", api.SignedOutSend); err != nil {
		t.Fatalf("send-anyway must deliver: %v", err)
	}
	if len(f.sent) != 1 || !strings.Contains(f.sent[0], "carry on") {
		t.Errorf("the message should have reached the session, got %q", f.sent)
	}
	if len(f.removed) != 0 {
		t.Errorf("send-anyway must not restart anything, got %q", f.removed)
	}
}

// TestTheAnswerOnlyAppliesToASignedOutPane: the field says what to do IF the agent reads signed
// out. Asked in advance — a CLI flag typed out of habit — it must not bounce a healthy session.
func TestTheAnswerOnlyAppliesToASignedOutPane(t *testing.T) {
	s, f := tellFixture(t, "nori", idlePane)
	if err := s.Tell("proj", "nori", "carry on", "user", api.SignedOutRestart); err != nil {
		t.Fatalf("a healthy agent takes the message as it always did: %v", err)
	}
	if len(f.removed) != 0 {
		t.Errorf("a pane that reads fine must not be restarted, got %q", f.removed)
	}
	if len(f.sent) != 1 {
		t.Errorf("the message should have been delivered once, got %q", f.sent)
	}
}

// TestRestartIsPerformedRatherThanRecommended: the old refusal told the user that a restart makes
// the process re-read the staged credentials, and then left them to it. Chosen, it is carried out
// — the pod goes down on the way to coming back up, and a failure says what it was doing.
func TestRestartIsPerformedRatherThanRecommended(t *testing.T) {
	s, f := tellFixture(t, "bombur", signedOutPane)
	err := s.Tell("proj", "bombur", "carry on", "user", api.SignedOutRestart)
	if len(f.removed) != 1 || f.removed[0] != "pod-bombur" {
		t.Errorf("the restart must tear the pod down first, got %q", f.removed)
	}
	// The relaunch cannot finish against a fake backend; what matters is that the failure is
	// reported as the restart it was, rather than as a message that quietly went nowhere.
	if err == nil || !strings.Contains(err.Error(), "restarting bombur") {
		t.Errorf("a failed restart must say so, got %v", err)
	}
}
