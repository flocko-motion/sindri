package fleet

import (
	"context"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"strings"
	"testing"
)

// TestRetiredWorkerIsHandedNothing is the point of the flag: an agent the user is winding down takes
// no further work, however much is waiting. The gate sits in ClaimNext so it holds whichever way the
// work would have arrived — the agent asking, or the hub waking it.
func TestRetiredWorkerIsHandedNothing(t *testing.T) {
	e, ps, c := retireFixture(t)
	flowtest.Retire(t, ps, "dvalin")

	if _, claimed, err := e.taskAct().ClaimNext(t.Context(), "proj", "dvalin"); err != nil || claimed {
		t.Fatalf("a retired worker must claim nothing: claimed=%v err=%v", claimed, err)
	}
	// The task is untouched and still open for somebody else.
	if task, _, _ := ps.GetTask("td-1"); task.Status != "open" {
		t.Errorf("the task should stay open for another worker, got %q", task.Status)
	}
	// And it is TOLD, rather than left blocking on a queue it is no longer served from.
	var out strings.Builder
	if code, err := e.taskAct().CmdNext(c, nil, &out); err != nil || code != 0 {
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
