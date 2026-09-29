package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/tools/paths"
)

// writeUsageOn lays a transcript reporting a size AND the model that carried it — the reading a live
// session has to be judged by, which writeUsage's model-less line cannot express.
func writeUsageOn(t *testing.T, project, name string, tokens int, model string) {
	t.Helper()
	dir := filepath.Join(paths.AgentHomeDir(project, name), "projects", "-workspace")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"assistant","message":{"model":"` + model + `","usage":{"input_tokens":` +
		itoa(tokens) + `,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "sess.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSetModelSwitchesASessionTheRecordAlreadyClaims is the bug a worker sat in for four days. The
// roster row already names the model and the session is running another: the record is the CHOICE,
// the transcript is the FACT, and a choice written down whose injection never landed leaves the two
// disagreeing. Reading the record as proof turns every later switch into a no-op reporting success.
//
// What that cost live: the worker's next child was rated mid, its session ran opus, and the hub
// retiered it every twelve seconds without ever sending the session anything.
func TestSetModelSwitchesASessionTheRecordAlreadyClaims(t *testing.T) {
	s, f := modelFixture(t)
	ps := s.store.For("proj")
	a, _, err := ps.GetAgent("durin")
	if err != nil {
		t.Fatal(err)
	}
	a.Model = "claude-opus-5" // written by a switch that never reached the session
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	writeUsageOn(t, "proj", "durin", 80_000, "claude-sonnet-5") // what it is actually running
	s.ForgetContext("proj", "durin")
	f.afterSubmit = func() {
		if len(f.sent) == 1 && f.sent[0] == "/clear" {
			writeUsageOn(t, "proj", "durin", 0, "claude-sonnet-5")
		}
	}

	if err := s.SetModel(t.Context(), "proj", "durin", "claude-opus-5"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}

	want := []string{"/clear", "/model claude-opus-5"}
	if len(f.sent) != len(want) {
		t.Fatalf("sent = %v, want %v — the record agreed, so nothing reached the session that did not", f.sent, want)
	}
	for i, w := range want {
		if f.sent[i] != w {
			t.Errorf("sent[%d] = %q, want %q", i, f.sent[i], w)
		}
	}
}
