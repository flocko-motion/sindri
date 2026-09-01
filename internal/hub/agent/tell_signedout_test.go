package agent

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/observe"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// signedOutPane is Claude's auth banner as the classifier matches it.
const signedOutPane = "● Login expired · Please run /login"

// idlePane is an ordinary prompt box — an agent that will take what it is told. DRAWN, at a real
// width: the needle is derived from the widest row, so a three-column stand-in would leave every
// fixture too narrow to confirm anything and quietly pass whatever it was asked.
var idlePane = idlePaneAt(paneWide)

// idlePaneAt is that box at a given terminal width, for the cases about width itself.
func idlePaneAt(cols int) string { return drawn(cols, "> ") }

// fakeRuntime answers for a container backend: what the pane says, what was typed into it, and
// which pods were torn down — the three facts these cases turn on.
type fakeRuntime struct {
	container.Runtime // every method the tests never reach
	pane              string
	sent              []string
	removed           []string
	interrupts        int
	submits           int
	typed             string // what SendLiteral typed and Submit has yet to send
	cols              int    // the width this fake terminal draws at; paneWide when unset
	blank             bool   // capture-pane comes back empty with no error: a reading that is not one
	blankFor          int    // ... for this many captures only, then normally: a pane blank in passing
	// swallow is a session that accepts the keystrokes and shows nothing — the failure the read-back
	// exists for, and the one thing a fake cannot be honest about by accident.
	swallow bool
	// afterSubmit runs once the Enter has gone, so a case about what a command does WHILE it waits
	// can change the world at that exact point instead of racing it with a sleep.
	afterSubmit func()
}

// termColumns is the fake terminal's OWN idea of how wide a rune draws, deliberately not the one
// under test: a fixture that borrowed production's answer would agree with it however wrong it was.
func termColumns(r rune) int {
	if r > 0x7F {
		return 2
	}
	return 1
}

// drawn is what a terminal DOES to typed text, which is the whole reason the needle is derived the
// way it is: rows wrapped at the box interior and split on newlines, each padded out behind full-width
// chrome. A fake that echoed the raw string back would match needles no real pane ever could — and
// width is a PARAMETER, since sindri creates panes from 9 columns up (-> tui.previewSize).
func drawn(width int, text string) string {
	inner := max(1, width-4)
	var b strings.Builder
	for line := range strings.SplitSeq(text, "\n") {
		for {
			row, used := "", 0
			for _, c := range line { // by COLUMNS, as a terminal wraps — not by runes
				if used+termColumns(c) > inner {
					break
				}
				row += string(c)
				used += termColumns(c)
			}
			b.WriteString("│ " + row + strings.Repeat(" ", inner-used) + " │\n")
			line = line[len(row):]
			if line == "" {
				break
			}
		}
	}
	return b.String()
}

func (f *fakeRuntime) Running(string) bool { return true }

// RunningContext answers as Running does: a liveness check that takes a deadline is asking the same
// question, and a fake that answered differently would make "alive" depend on which one a path used.
func (f *fakeRuntime) RunningContext(context.Context, string) bool { return true }

func (f *fakeRuntime) Exec(name string, args ...string) ([]byte, error) {
	return f.ExecContext(context.Background(), name, args...)
}

// paneWide is the default fake width, roomy enough that a case not about width never turns on it.
const paneWide = 76

func (f *fakeRuntime) width() int {
	if f.cols > 0 {
		return f.cols
	}
	return paneWide
}

// ExecContext honours ctx, as a real pod exec does — otherwise a test about an abandoned caller
// would be answered by a runtime that never noticed the caller was gone.
func (f *fakeRuntime) ExecContext(ctx context.Context, _ string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch {
	case containsArg(args, "capture-pane"):
		if f.blankFor > 0 {
			f.blankFor--
			return nil, nil
		}
		if f.blank {
			return nil, nil
		}
		return []byte(f.pane), nil
	case containsArg(args, "send-keys") && containsArg(args, "-l"):
		text := args[len(args)-1]
		f.sent = append(f.sent, text)
		f.typed = text
		if !f.swallow {
			f.pane += drawn(f.width(), text)
		}
	case containsArg(args, "send-keys") && containsArg(args, "Enter"):
		f.submits++
		// /clear is the case the read-back's POSITION exists for: submitting it wipes the transcript
		// the text was echoed into, so a capture taken after this Enter can never find it.
		if strings.TrimSpace(f.typed) == "/clear" {
			f.pane = idlePane
		}
		f.typed = ""
		if f.afterSubmit != nil {
			f.afterSubmit()
		}
	case containsArg(args, "send-keys") && containsArg(args, "Escape"):
		f.interrupts++
	}
	return nil, nil
}

func (f *fakeRuntime) Rm(name string) error { return f.RmContext(context.Background(), name) }

// RmContext records the removal as Rm does: a teardown that takes a deadline still tears the same
// pod down, and a fake that only counted one of the two would miss whichever verb a path used.
func (f *fakeRuntime) RmContext(_ context.Context, name string) error {
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
func (tellDeps) Kickoff(_, _ string) string                  { return "[hub] kickoff" }
func (tellDeps) ForgetFill(_, _ string)                      {}

func (tellDeps) Deliver(_, _, _ string, _ workflow.Delivery) error { return nil }

// Observation mirrors the always-up fake runtime this fixture backs, so the situation-derived rules
// see the same liveness AgentUp reports.
func (tellDeps) Observation(_, _ string) observe.Observation {
	return observe.Observation{TakenAt: time.Now(), Up: true}
}

// AgentUp mirrors fakeRuntime's always-up container, so the idle/clear sweeps this fixture backs
// see the same liveness AgentAlive would have probed. AgentClients: no test here dials in a human.
func (tellDeps) AgentUp(_, _ string) bool     { return true }
func (tellDeps) AgentClients(_, _ string) int { return 0 }

// paneReader is a coding-agent backend that reads the one banner these cases turn on. The port
// defaults to a no-op that classifies nothing, and what the real classifier makes of a screen is
// its own package's business (-> adapter/agent/claude); here the question is what the hub does with
// the verdict.
type paneReader struct{ agentport.Agent }

func (paneReader) DetectState(screen string) agentport.State {
	switch s := strings.ToLower(screen); {
	case strings.Contains(s, "please run /login"):
		return agentport.SignedOut
	case strings.Contains(s, "do you want to proceed?"):
		return agentport.Blocked
	}
	return agentport.Idle
}

func (paneReader) ToolRunning(string) bool { return false }

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
	return s, f
}

// unreadablePane restores the port to saying nothing about any screen.
type unreadablePane struct{ agentport.Agent }

func (unreadablePane) DetectState(string) agentport.State { return agentport.Unknown }

// TestAHubMessageStillRefusesASignedOutPane is the half of the guard that must survive: a verdict
// or an assignment has no human behind it to notice that it landed in an input box unread, so it
// fails loudly instead of vanishing — and the refusal names the restart that fixes it.
func TestAHubMessageStillRefusesASignedOutPane(t *testing.T) {
	s, f := tellFixture(t, "eitri", signedOutPane)
	err := s.Inject(t.Context(), "proj", "eitri", "the verdict is in")
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
	if err := s.Tell(t.Context(), "proj", "dvalin", "carry on", "user", api.SignedOutSend); err != nil {
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
	if err := s.Tell(t.Context(), "proj", "nori", "carry on", "user", api.SignedOutRestart); err != nil {
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
	err := s.Tell(t.Context(), "proj", "bombur", "carry on", "user", api.SignedOutRestart)
	if len(f.removed) != 1 || f.removed[0] != "pod-bombur" {
		t.Errorf("the restart must tear the pod down first, got %q", f.removed)
	}
	// The relaunch cannot finish against a fake backend; what matters is that the failure is
	// reported as the restart it was, rather than as a message that quietly went nowhere.
	if err == nil || !strings.Contains(err.Error(), "restarting bombur") {
		t.Errorf("a failed restart must say so, got %v", err)
	}
}
