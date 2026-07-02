package hub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testProject = "proj"

// newHub opens a global hub rooted at a temp state dir (via SINDRI_HOME), so tests
// never touch the real ~/.local/state/sindri.
func newHub(t *testing.T) *Hub {
	t.Helper()
	t.Setenv("SINDRI_HOME", t.TempDir())
	h, err := New()
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

func TestEnsureGitignore(t *testing.T) {
	count := func(s, sub string) int { return strings.Count(s, sub) }

	// ensureGitignore adds the hub's in-repo artifacts (now just .worktrees/).
	root := t.TempDir()
	ensureGitignore(root)
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
	if c := count(string(again), ".worktrees/"); c != 1 {
		t.Errorf("expected .worktrees/ once, got %d", c)
	}

	// Existing entries (any slash form) are respected, not duplicated; .todos stays
	// untouched (it is tracked). .sindri is no longer written (hub state is central).
	root2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(root2, ".gitignore"), []byte("node_modules\n/.worktrees\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ensureGitignore(root2)
	out, _ := os.ReadFile(filepath.Join(root2, ".gitignore"))
	if c := count(string(out), ".worktrees"); c != 1 {
		t.Errorf("existing /.worktrees should not be duplicated, got %d:\n%s", c, out)
	}
	if strings.Contains(string(out), ".todos") {
		t.Errorf(".todos must not be ignored (it is tracked):\n%s", out)
	}
	if strings.Contains(string(out), ".sindri") {
		t.Errorf(".sindri must not be written (hub state is central now):\n%s", out)
	}
}

func TestNewAgentValidation(t *testing.T) {
	h := newHub(t)
	if _, err := h.NewAgent(testProject, "Brokkr", "worker"); err == nil {
		t.Fatalf("uppercase name should be rejected")
	}
	if _, err := h.NewAgent(testProject, "brokkr", "boss"); err == nil {
		t.Fatalf("bad role should be rejected")
	}
	if _, err := h.NewAgent(testProject, "brokkr", "worker"); err != nil {
		t.Fatalf("valid agent: %v", err)
	}
	if _, err := h.NewAgent(testProject, "brokkr", "worker"); err == nil {
		t.Fatalf("duplicate agent should be rejected")
	}
}

func TestNewAgentAutoName(t *testing.T) {
	h := newHub(t)
	n1, err := h.NewAgent(testProject, "", "worker")
	if err != nil {
		t.Fatal(err)
	}
	if n1 != dwarfNames[0] {
		t.Fatalf("first auto-name = %q, want %q", n1, dwarfNames[0])
	}
	n2, err := h.NewAgent(testProject, "", "worker")
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
	if _, err := h.NewAgent(testProject, "dvalin", "reviewer"); err != nil {
		t.Fatal(err)
	}
	st, err := h.State(testProject)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Agents) != 1 || st.Agents[0].Name != "dvalin" || st.Agents[0].Role != "reviewer" {
		t.Fatalf("unexpected state: %+v", st)
	}
	if st.Agents[0].Status != "down" { // podman absent → session not alive
		t.Fatalf("expected status down, got %q", st.Agents[0].Status)
	}
	evs, _ := h.store.For(testProject).Events("dvalin", 0)
	if len(evs) != 1 || evs[0].Type != "register" {
		t.Fatalf("register not logged: %+v", evs)
	}
}

func TestTellUnknownAgent(t *testing.T) {
	h := newHub(t)
	if err := h.Tell(testProject, "ghost", "hi", "user"); err == nil {
		t.Fatalf("telling unknown agent should error")
	}
}
