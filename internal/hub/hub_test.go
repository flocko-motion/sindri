package hub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureGitignore(t *testing.T) {
	count := func(s, sub string) int { return strings.Count(s, sub) }

	// Fresh repo: New() (called by newHub) creates .gitignore with the hub patterns.
	root := t.TempDir()
	if _, err := New(root); err != nil {
		t.Fatalf("new: %v", err)
	}
	gi := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(gi)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	got := string(data)
	for _, want := range hubIgnores {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in .gitignore:\n%s", want, got)
		}
	}

	// Idempotent: a second pass adds nothing.
	ensureGitignore(root)
	again, _ := os.ReadFile(gi)
	if string(again) != got {
		t.Errorf("ensureGitignore not idempotent:\n--- first ---\n%s\n--- second ---\n%s", got, again)
	}
	if c := count(string(again), ".sindri/"); c != 1 {
		t.Errorf("expected .sindri/ once, got %d", c)
	}

	// Existing entries (any slash form) are respected, not duplicated; .todos stays
	// untouched (it is tracked).
	root2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(root2, ".gitignore"), []byte("node_modules\n/.sindri\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ensureGitignore(root2)
	out, _ := os.ReadFile(filepath.Join(root2, ".gitignore"))
	if c := count(string(out), ".sindri"); c != 1 {
		t.Errorf("existing /.sindri should not be duplicated, got %d occurrences:\n%s", c, out)
	}
	if !strings.Contains(string(out), ".worktrees/") {
		t.Errorf(".worktrees/ should have been added:\n%s", out)
	}
	if strings.Contains(string(out), ".todos") {
		t.Errorf(".todos must not be ignored (it is tracked):\n%s", out)
	}
}

func newHub(t *testing.T) *Hub {
	t.Helper()
	h, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

func TestNewAgentValidation(t *testing.T) {
	h := newHub(t)
	if _, err := h.NewAgent("Brokkr", "worker"); err == nil {
		t.Fatalf("uppercase name should be rejected")
	}
	if _, err := h.NewAgent("brokkr", "boss"); err == nil {
		t.Fatalf("bad role should be rejected")
	}
	if _, err := h.NewAgent("brokkr", "worker"); err != nil {
		t.Fatalf("valid agent: %v", err)
	}
	if _, err := h.NewAgent("brokkr", "worker"); err == nil {
		t.Fatalf("duplicate agent should be rejected")
	}
}

func TestNewAgentAutoName(t *testing.T) {
	h := newHub(t)
	// Empty name ⇒ first unused dwarf name.
	n1, err := h.NewAgent("", "worker")
	if err != nil {
		t.Fatal(err)
	}
	if n1 != dwarfNames[0] {
		t.Fatalf("first auto-name = %q, want %q", n1, dwarfNames[0])
	}
	// Next one skips the taken name.
	n2, err := h.NewAgent("", "worker")
	if err != nil {
		t.Fatal(err)
	}
	if n2 != dwarfNames[1] {
		t.Fatalf("second auto-name = %q, want %q", n2, dwarfNames[1])
	}
	if n2 == "sindri" || n1 == "sindri" {
		t.Fatalf("must never hand out 'sindri'")
	}
}

func TestNewAgentRecordsIdentityAndLog(t *testing.T) {
	h := newHub(t)
	if _, err := h.NewAgent("dvalin", "reviewer"); err != nil {
		t.Fatal(err)
	}
	st, err := h.State()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Agents) != 1 || st.Agents[0].Name != "dvalin" || st.Agents[0].Role != "reviewer" {
		t.Fatalf("unexpected state: %+v", st)
	}
	if st.Agents[0].Status != "down" { // podman absent → session not alive
		t.Fatalf("expected status down, got %q", st.Agents[0].Status)
	}
	// register event logged
	evs, _ := h.store.Events("dvalin", 0)
	if len(evs) != 1 || evs[0].Type != "register" {
		t.Fatalf("register not logged: %+v", evs)
	}
}

func TestTellUnknownAgent(t *testing.T) {
	h := newHub(t)
	if err := h.Tell("ghost", "hi", "user"); err == nil {
		t.Fatalf("telling unknown agent should error")
	}
}
