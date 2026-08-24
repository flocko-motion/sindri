package agent

import (
	"os"
	"path/filepath"
	"testing"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/agent/claude"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/tools/paths"
)

// writeUsage lays a Claude transcript in an agent's home reporting exactly these input tokens, so a
// reading can be changed between two calls the way a real /clear changes it.
func writeUsage(t *testing.T, project, name string, tokens int) {
	t.Helper()
	dir := filepath.Join(paths.AgentHomeDir(project, name), "projects", "-workspace")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"assistant","message":{"usage":{"input_tokens":` +
		itoa(tokens) + `,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "sess.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// TestTheClearItselfDropsTheStaleReading closes the gap this change reported rather than implied:
// every other test invalidates the memo directly, so removing the call from the clear passed. It
// runs the real thing — a live pane to type into, a transcript on disk to measure — and asks the
// question the bug asked: after clearing, does the hub still answer from the pre-clear figure?
//
// That figure is why a cleared agent was told "past the point of taking on new work" by the very
// kickoff the clear had just sent it: ContextUsage memoises for contextTTL, the kickoff lands well
// inside that window, and the reading survived the act that made it false.
func TestTheClearItselfDropsTheStaleReading(t *testing.T) {
	t.Setenv("SINDRI_HOME", t.TempDir()) // AgentHomeDir reads this, so the transcript is ours
	_, st := newService(t)
	s := New(st, tellDeps{}, nil)
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	container.Use(&fakeRuntime{pane: idlePane}) // a running pod with a pane to type into
	agentport.Use(claude.New())                 // the real transcript reader, over our own files
	t.Cleanup(func() {
		container.UseDefault()
		agentport.Use(unreadablePane{})
	})

	writeUsage(t, "proj", "eitri", 900_000)
	if got, _, _, ok := s.ContextUsage("proj", "eitri"); !ok || got != 900_000 {
		t.Fatalf("ContextUsage = (%d, %v), want the full reading — the memo must hold it first", got, ok)
	}

	// The clear: /clear into the session, and the transcript it leaves behind is a fresh one.
	writeUsage(t, "proj", "eitri", 1_000)
	if err := s.SetClearArmed(t.Context(), "proj", "eitri", true); err != nil {
		t.Fatalf("clearing an idle agent at a boundary: %v", err)
	}

	got, _, _, ok := s.ContextUsage("proj", "eitri")
	if !ok {
		t.Fatal("the reading went missing entirely, rather than being re-taken")
	}
	if got == 900_000 {
		t.Fatal("the clear served the pre-clear reading back: the memo outlived the act that " +
			"invalidated it, which is what told a cleared agent it was still full")
	}
	if got != 1_000 {
		t.Errorf("ContextUsage = %d, want 1000 — the reading taken after the clear", got)
	}
}
