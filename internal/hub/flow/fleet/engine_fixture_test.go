package fleet

import (
	"bytes"
	"context"
	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/adapter/tasks"
	flowpr "github.com/flo-at/sindri/internal/hub/flow/pr"
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

// submitAll drives a whole submit the way an agent does: the `submit` that asks for one, then an
// answer per question, each arriving as its own `submit` call — and then the taking, which the
// worker's own map does in production (-> worker/submitting) and which is called here directly, so
// a test about what follows a submit does not have to wait on an interview.
//
// A test whose subject is the INTERVIEW drives the machine instead (-> flowmachine_interview_test.go).
func submitAll(t *testing.T, e *Engine, c registry.Caller, summary string) (int, string) {
	t.Helper()
	const answer = "I swept the call sites this touches and each one is covered by a test that fails without it."
	ps := e.Store.For(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		t.Fatal(err)
	}
	var code int
	var out bytes.Buffer
	text := summary
	for i := 0; i < 6; i++ { // a bound, so a flow that never settles fails loudly rather than hanging
		out.Reset()
		if code, err = e.prAct().CmdSubmit(c, []string{text}, &out); err != nil {
			t.Fatalf("CmdSubmit: %v", err)
		}
		if code != 0 {
			return code, out.String() // refused before any interview opened
		}
		if _, ok, aerr := ps.SubmitAsked(c.Agent); aerr != nil {
			t.Fatalf("SubmitAsked: %v", aerr)
		} else if !ok {
			return code, out.String() // no submit was asked for, so there is nothing to take
		}
		_, rows, rerr := ps.OpenSubmitAnswers(c.Agent)
		if rerr != nil {
			t.Fatalf("OpenSubmitAnswers: %v", rerr)
		}
		if !flowpr.InterviewOpen(rows) {
			if _, _, terr := e.prAct().TakeSubmit(c.Project, c.Agent); terr != nil {
				t.Fatalf("TakeSubmit: %v", terr)
			}
			// Where that act's outcome leads, since this stood in for the state that runs it: the
			// submit is queued, so the author waits at the gate (-> worker/submitting's act.Queued).
			flowtest.Place(t, ps, store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch,
				Container: st.Container, Phase: worker.Gating})
			return code, out.String()
		}
		text = answer
	}
	t.Fatal("the submit questions never finished")
	return 0, ""
}

// runQueuedGate takes the gate a submit or contribute just queued, runs it, and settles whoever was
// waiting on it. The settle is the half a test forgets: the gate's result is a fact, and the author
// standing at it is moved by its own map reading that — a beat away in production, a look here.
func runQueuedGate(t *testing.T, e *Engine) {
	t.Helper()
	project, id, ok := e.runAct().NextQueuedRun()
	if !ok {
		t.Fatal("expected a queued gate run")
	}
	r, _, _ := e.Store.For(project).GetRun(id)
	if err := e.runAct().ExecuteRun(t.Context(), project, id); err != nil {
		t.Fatalf("ExecuteRun(%s): %v", id, err)
	}
	if r.Agent != "" {
		e.Look(project, r.Agent)
	}
}
