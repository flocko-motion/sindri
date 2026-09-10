package fleet

import (
	"bytes"
	"context"
	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/adapter/tasks"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// stubDeps is the fake hub every subject's tests share (-> hub/flowtest), named here as the fleet's
// tests have always named it.
type stubDeps = flowtest.Hub

// saying and sayingWhileDown are readings of a pane, for a test that judges on one.
var saying = flowtest.Saying
var sayingWhileDown = flowtest.SayingWhileDown

// newEngine wires ONE stub as both halves of the split seam, so a test still asserts against a
// single recorder — the split is about who may call what, not about having two fixtures.
func newEngine(t *testing.T, st *store.Store, d *stubDeps, sources ...tasks.Source) *Engine {
	t.Helper()
	// A fixture with no root hands out paths relative to the package directory, which is how a git
	// repo came to be written into internal/hub/flow/fleet. One is given rather than demanded.
	if d.Root == "" {
		d.Root = t.TempDir()
	}
	// Background as the lifetime: a test IS an entrypoint, and no fixture here turns on the engine
	// outliving it. A case that needs the lifetime cancelled builds its own.
	return New(context.Background(), st, d, d, mail.New(st, d), sources...)
}

// storelessEngine is for a case that exercises the harness side alone. It still gets a real store —
// a running action is a STATE now, so even preparing a session writes a phase, and a fixture with no
// store would panic rather than test anything.
func storelessEngine(t *testing.T, d *stubDeps, sources ...tasks.Source) *Engine {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return newEngine(t, st, d, sources...)
}

// submitAll drives CmdSubmit through its questions the way an agent does: the summary, then an
// answer per question, each arriving as its own `submit` call. Returns the final call's code and
// output — the one that either lands the submission or refuses it.
//
// A test that wants to see a QUESTION calls CmdSubmit directly; this is for the tests whose subject
// is what happens after the submit is taken.
func submitAll(t *testing.T, e *Engine, c registry.Caller, summary string) (int, string) {
	t.Helper()
	const answer = "I swept the call sites this touches and each one is covered by a test that fails without it."
	text := summary
	for i := 0; i < 6; i++ { // a bound, so a flow that never settles fails loudly rather than hanging
		var out bytes.Buffer
		code, err := e.prAct().CmdSubmit(c, []string{text}, &out)
		if err != nil {
			t.Fatalf("CmdSubmit: %v", err)
		}
		// Both shapes mean the questionnaire is still running: a question put, or one put again
		// because the last answer was too short to be one.
		if !strings.Contains(out.String(), "Before this submit is taken") &&
			!strings.Contains(out.String(), "too short") {
			return code, out.String()
		}
		text = answer
	}
	t.Fatal("the submit questions never finished")
	return 0, ""
}

// runQueuedGate takes the gate a submit or contribute just queued and runs it.
func runQueuedGate(t *testing.T, e *Engine) {
	t.Helper()
	project, id, ok := e.runAct().NextQueuedRun()
	if !ok {
		t.Fatal("expected a queued gate run")
	}
	if err := e.runAct().ExecuteRun(t.Context(), project, id); err != nil {
		t.Fatalf("ExecuteRun(%s): %v", id, err)
	}
}
