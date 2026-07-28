package workflow

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// plannerEngine builds an engine over a throwaway store with one cached task in the given
// approval state, and returns the caller a planner would arrive as.
func plannerEngine(t *testing.T, id, approval string) (*Engine, registry.Caller, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("proj", t.TempDir()); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("proj")
	if err := ps.UpsertTask(store.Task{ID: id, Title: "flat proposal", Status: "open"}); err != nil {
		t.Fatalf("upsert task: %v", err)
	}
	if approval != "" {
		if err := ps.SetApproval(id, approval, ""); err != nil {
			t.Fatalf("set approval: %v", err)
		}
	}
	e := New(st, &stubDeps{root: t.TempDir()})
	return e, registry.Caller{Project: "proj", Agent: "galar", Role: "planner"}, ps
}

// TestEditTaskRefusedOnceApproved is the constraint the approval gate exists for: approval is
// the user's decision to take the task as it stands, so what a worker picks up is what the
// user read and released.
func TestEditTaskRefusedOnceApproved(t *testing.T) {
	for _, approval := range []string{"approved", "rejected", ""} {
		e, c, _ := plannerEngine(t, "td-1", approval)
		var out bytes.Buffer
		code, err := e.CmdEditTask(c, []string{"td-1", "--parent", "td-9"}, &out)
		if err != nil {
			t.Fatalf("approval %q: unexpected error: %v", approval, err)
		}
		if code == 0 {
			t.Errorf("approval %q: edit should be refused, got exit 0: %s", approval, out.String())
		}
		if !strings.Contains(out.String(), "awaiting the user's approval") {
			t.Errorf("approval %q: refusal should state the rule, got: %s", approval, out.String())
		}
	}
}

// TestEditTaskUsageIsVisible: a malformed call answers with usage on the agent's own stream.
// A returned error would reach it as "the hub hit an internal error", which it cannot act on.
func TestEditTaskUsageIsVisible(t *testing.T) {
	e, c, _ := plannerEngine(t, "td-1", "pending")
	for _, args := range [][]string{{}, {"td-1"}, {"td-1", "--nope", "x"}} {
		var out bytes.Buffer
		code, err := e.CmdEditTask(c, args, &out)
		if err != nil {
			t.Fatalf("args %v: usage must not be an error: %v", args, err)
		}
		if code != 2 {
			t.Errorf("args %v: exit = %d, want 2", args, code)
		}
		if !strings.Contains(out.String(), "usage: edit-task") {
			t.Errorf("args %v: expected usage, got: %s", args, out.String())
		}
	}
}

// TestPlannerCannotSetPriority: OpenLeaves hands out only tasks that are approved AND carry a
// priority, so the priority a human sets at approval is what releases work. A planner setting
// it would hand itself the release switch.
func TestPlannerCannotSetPriority(t *testing.T) {
	for _, flag := range []string{"--priority", "-p"} {
		if _, _, err := parseTaskFlags([]string{flag, "P0", "urgent thing"}); err == nil {
			t.Errorf("%s should be refused, not accepted", flag)
		}
	}
	if strings.Contains(createTaskUsage, "--priority") {
		t.Error("create-task usage should not advertise --priority")
	}
}

// TestUnknownFlagRefused: a silently ignored flag looks like it took effect, and the task is
// then created or edited without the parent or body that was asked for.
func TestUnknownFlagRefused(t *testing.T) {
	if _, _, err := parseTaskFlags([]string{"--tree"}); err == nil {
		t.Error("an unknown flag should be an error the caller sees")
	}
	if _, _, err := parseTaskFlags([]string{"--parent"}); err == nil {
		t.Error("a flag with no value should be an error")
	}
}

// TestParseTaskFlagsFormsAndTitle: both `--flag value` and `--flag=value`, with the leftover
// words forming the title.
func TestParseTaskFlagsFormsAndTitle(t *testing.T) {
	s, words, err := parseTaskFlags([]string{"--parent=os-adc678", "--type", "feature", "--body", "why it matters", "wire", "the", "thing"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Parent != "os-adc678" || s.Type != "feature" || s.Description != "why it matters" {
		t.Errorf("flags parsed as %+v", s)
	}
	if got := strings.Join(words, " "); got != "wire the thing" {
		t.Errorf("title words = %q", got)
	}
}

// TestSyncToleratesRepoWithoutTd: a repo that tracks work only in openspec or GitHub issues
// must still list those. td gates on having a store, so its absence leaves it with nothing to
// contribute instead of failing the sync for every source — which is what made the host CLI
// error with "no td store" while the TUI, reading the cache, happily showed the same repo's
// tasks. One source's absence must not decide the answer for all of them.
func TestSyncToleratesRepoWithoutTd(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir() // no .todos, no openspec/, no GitHub remote
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	e := New(st, &stubDeps{root: root})

	if err := e.SyncTasks("proj"); err != nil {
		t.Fatalf("a repo with no td store should sync cleanly, got: %v", err)
	}
	// And the read path the host CLI uses must agree, rather than surfacing an error the
	// board never sees.
	if _, err := e.Tasks("proj"); err != nil {
		t.Fatalf("Tasks should succeed with no td store, got: %v", err)
	}
}
