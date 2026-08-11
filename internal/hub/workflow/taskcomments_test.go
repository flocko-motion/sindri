package workflow

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestAgentSeesCommentsOnTaskInfo is the DONE WHEN this task names: an agent that files a finding
// via the comment verb has to be able to read it back through `task <id>` — otherwise the verb is
// worse than none, since it looks like the finding landed somewhere nobody will ever see it.
func TestAgentSeesCommentsOnTaskInfo(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("proj")
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-9", Title: "a task", Status: "open", Priority: "P1"}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: "td-9", Phase: "working"}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	deps := &stubDeps{root: root, comments: []store.Comment{
		{Source: "sindri", SourceRef: "abc", Author: "eitri", Body: "found a blocker", CreatedAt: "2026-01-01T00:00:00Z"},
	}}
	e := New(st, deps)
	c := registry.Caller{Project: "proj", Agent: "eitri", Role: "worker"}

	var out bytes.Buffer
	if _, err := e.CmdTasks(c, []string{"td-9"}, &out); err != nil {
		t.Fatalf("CmdTasks: %v", err)
	}
	got := out.String()
	for _, want := range []string{"eitri", "sindri", "found a blocker"} {
		if !strings.Contains(got, want) {
			t.Errorf("task info output missing %q:\n%s", want, got)
		}
	}
}

// TestAgentSeesNoCommentsSectionWhenThereAreNone: an empty thread adds nothing to read past.
func TestAgentSeesNoCommentsSectionWhenThereAreNone(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("proj")
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-9", Title: "a task", Status: "open", Priority: "P1"}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: "td-9", Phase: "working"}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	e := New(st, &stubDeps{root: root})
	c := registry.Caller{Project: "proj", Agent: "eitri", Role: "worker"}

	var out bytes.Buffer
	if _, err := e.CmdTasks(c, []string{"td-9"}, &out); err != nil {
		t.Fatalf("CmdTasks: %v", err)
	}
	if strings.Contains(out.String(), "—") {
		t.Errorf("no comment thread should print no comment lines:\n%s", out.String())
	}
}
