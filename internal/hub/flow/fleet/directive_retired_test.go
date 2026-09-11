package fleet

import (
	"context"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"strings"
	"testing"
)

// TestRetiredWorkerIsHandedNothing is the point of the flag: an agent the user is winding down takes
// no further work, however much is waiting. The gate is a state — retirement comes before every
// route into a claim on the worker's own map — so it holds whichever way the work would have
// arrived: the agent asking, or the hub deciding on its own beat.
func TestRetiredWorkerIsHandedNothing(t *testing.T) {
	e, ps, c := retireFixture(t)
	flowtest.Retire(t, ps, "dvalin")

	e.Look("proj", "dvalin")

	if st, _ := ps.GetState("dvalin"); st.Task != "" {
		t.Errorf("a retired worker was put on %q — nothing further is assigned to it", st.Task)
	}
	// The task is untouched and still open for somebody else.
	if task, _, _ := ps.GetTask("td-1"); task.Status != "open" {
		t.Errorf("the task should stay open for another worker, got %q", task.Status)
	}
	// And it is TOLD, rather than left blocking on a queue it is no longer served from.
	var out strings.Builder
	if code, err := e.CmdNext(c, nil, &out); err != nil || code != 0 {
		t.Fatalf("CmdNext: code=%d err=%v", code, err)
	}
	if !strings.Contains(out.String(), "retired") {
		t.Errorf("a retired worker should be told why it gets nothing: %q", out.String())
	}
	// The blocking path answers at once too — waiting forever is what "no tasks" would have meant.
	d, err := e.AgentDirective(context.Background(), "proj", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(d, "retired") {
		t.Errorf("the directive should say it is retired, got %q", d)
	}
}

// TestUnretiredWorkerIsServedAgain: winding down is reversible, and the refusal goes with the flag —
// nothing about the agent or the backlog was consumed while it was set.
func TestUnretiredWorkerIsServedAgain(t *testing.T) {
	e, ps, c := retireFixture(t)
	flowtest.Retire(t, ps, "dvalin")
	var out strings.Builder
	if _, err := e.CmdNext(c, nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "retired") {
		t.Fatalf("setup: expected the retirement refusal, got %q", out.String())
	}

	ag, _, _ := ps.GetAgent("dvalin")
	ag.Retired = false
	if err := ps.PutAgent(ag); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if _, err := e.CmdNext(c, nil, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "retired") {
		t.Errorf("back in service, it must not still be refused: %q", out.String())
	}
}
