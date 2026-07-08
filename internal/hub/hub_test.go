package hub

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
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

	// Existing entries (any slash form) are respected, not duplicated; .todos/ is
	// added (a tracked task DB breaks the hub's merge flow). .sindri is no longer
	// written (hub state is central).
	root2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(root2, ".gitignore"), []byte("node_modules\n/.worktrees\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ensureGitignore(root2)
	out, _ := os.ReadFile(filepath.Join(root2, ".gitignore"))
	if c := count(string(out), ".worktrees"); c != 1 {
		t.Errorf("existing /.worktrees should not be duplicated, got %d:\n%s", c, out)
	}
	if !strings.Contains(string(out), ".todos") {
		t.Errorf(".todos/ should be ignored (the hub's merge flow needs a clean tree):\n%s", out)
	}
	if strings.Contains(string(out), ".sindri") {
		t.Errorf(".sindri must not be written (hub state is central now):\n%s", out)
	}
}

func TestNewAgentValidation(t *testing.T) {
	h := newHub(t)
	if _, err := h.NewAgent(testProject, "Brokkr", "worker", ""); err == nil {
		t.Fatalf("uppercase name should be rejected")
	}
	if _, err := h.NewAgent(testProject, "brokkr", "boss", ""); err == nil {
		t.Fatalf("bad role should be rejected")
	}
	if _, err := h.NewAgent(testProject, "brokkr", "worker", ""); err != nil {
		t.Fatalf("valid agent: %v", err)
	}
	if _, err := h.NewAgent(testProject, "brokkr", "worker", ""); err == nil {
		t.Fatalf("duplicate agent should be rejected")
	}
}

func TestNewAgentAutoName(t *testing.T) {
	h := newHub(t)
	isDwarf := func(n string) bool {
		for _, d := range dwarfNames {
			if d == n {
				return true
			}
		}
		return false
	}
	n1, err := h.NewAgent(testProject, "", "worker", "")
	if err != nil {
		t.Fatal(err)
	}
	// A different project must still get a globally-unique name (not reuse n1).
	n2, err := h.NewAgent("other", "", "worker", "")
	if err != nil {
		t.Fatal(err)
	}
	if !isDwarf(n1) || !isDwarf(n2) {
		t.Fatalf("auto-names should be dwarves: %q, %q", n1, n2)
	}
	if n1 == n2 {
		t.Fatalf("auto-names must be globally unique across projects, got %q twice", n1)
	}
	if n1 == "sindri" || n2 == "sindri" || n1 == "brokkr" || n2 == "brokkr" {
		t.Fatalf("must never hand out a binary name")
	}
}

func TestNewAgentNameGloballyUnique(t *testing.T) {
	h := newHub(t)
	if _, err := h.NewAgent("repoA", "eitri", "worker", ""); err != nil {
		t.Fatal(err)
	}
	// The same name in a DIFFERENT repo is refused — names are unique machine-wide.
	if _, err := h.NewAgent("repoB", "eitri", "worker", ""); err == nil {
		t.Fatalf("same name in another repo should be rejected (global uniqueness)")
	}
}

func TestNewAgentRecordsIdentityAndLog(t *testing.T) {
	h := newHub(t)
	if _, err := h.NewAgent(testProject, "dvalin", "reviewer", ""); err != nil {
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

// TestEnsureArchitectureDoc: the hub seeds a placeholder ARCHITECTURE.md into a
// repo that has none (pointing the user at the brokkr baseline), and never
// overwrites an existing one.
func TestEnsureArchitectureDoc(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ARCHITECTURE.md")

	ensureArchitectureDoc(root)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected a seeded ARCHITECTURE.md: %v", err)
	}
	if !strings.Contains(string(data), "brokkr") {
		t.Errorf("seed should mention the brokkr linter baseline:\n%s", data)
	}

	// Idempotent + non-destructive: an existing doc is left untouched.
	if err := os.WriteFile(path, []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ensureArchitectureDoc(root)
	again, _ := os.ReadFile(path)
	if string(again) != "# mine\n" {
		t.Errorf("existing ARCHITECTURE.md must not be overwritten, got:\n%s", again)
	}
}

// TestReviewInstructionsCarryArchitecture: both review-instruction paths (the no-arg
// `sindri` directive = dirReview, and the injected = msgReview) always tell the
// reviewer to read the repo's ARCHITECTURE.md.
func TestReviewInstructionsCarryArchitecture(t *testing.T) {
	if !strings.Contains(dirReview("pr-1", "td-1", "ARCHITECTURE.md"), "ARCHITECTURE.md") {
		t.Errorf("dirReview must tell the reviewer to read the architecture doc")
	}
	if !strings.Contains(msgReview("pr-1", "req", "br", "base", "ARCHITECTURE.md", true), "ARCHITECTURE.md") {
		t.Errorf("msgReview must tell the reviewer to read the architecture doc")
	}
}

func TestTellUnknownAgent(t *testing.T) {
	h := newHub(t)
	if err := h.Tell(testProject, "ghost", "hi", "user"); err == nil {
		t.Fatalf("telling unknown agent should error")
	}
}

// tdCreate runs `td -w <root> create <args...>`, failing the test on error.
func tdCreate(t *testing.T, root string, args ...string) {
	t.Helper()
	full := append([]string{"-w", root, "create"}, args...)
	if out, err := exec.Command("td", full...).CombinedOutput(); err != nil {
		t.Fatalf("td create %v: %s", args, out)
	}
}

// TestRefreshSyncsTasksFromTd: Refresh re-syncs the hub's task cache from td's
// store (the source of truth), so a task td gains after the first sync shows up
// on the next Refresh. Skips when the td CLI isn't installed (matching the td
// adapter's tests) — the read path is td's SQLite db.
func TestRefreshSyncsTasksFromTd(t *testing.T) {
	if _, err := exec.LookPath("td"); err != nil {
		t.Skip("td CLI not installed")
	}
	h := newHub(t)
	root := t.TempDir()
	if out, err := exec.Command("td", "-w", root, "init").CombinedOutput(); err != nil {
		t.Fatalf("td init: %s", out)
	}
	ps := h.repo(root) // register the project → its tag; seeds the cache
	tag := RepoTag(root)

	tdCreate(t, root, "-t", "feature", "First task in the backlog")
	if err := h.Refresh(tag); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	after, _ := ps.AllTasks()
	if !hasTaskTitled(after, "First task in the backlog") {
		t.Fatalf("first task not synced from td: %+v", after)
	}

	// A task td gains later must appear on the next Refresh — the point of the sync.
	tdCreate(t, root, "-t", "bug", "Second task added later on")
	if err := h.Refresh(tag); err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	after, _ = ps.AllTasks()
	if !hasTaskTitled(after, "Second task added later on") {
		t.Fatalf("task added after the first sync was not picked up on refresh: %+v", after)
	}
}

func hasTaskTitled(tasks []store.Task, title string) bool {
	for _, t := range tasks {
		if t.Title == title {
			return true
		}
	}
	return false
}

// TestCloseUnresolvableOpenspec covers the agnostic dispatch's failure path: closing
// is routed by the task's backend, and an os- id that resolves to no known openspec
// change (here a bogus one, with no openspec/ dir) errors clearly rather than
// silently doing nothing. (A real os- close archives the change; that needs the
// openspec CLI + a change, so it's exercised end-to-end, not here.)
func TestCloseUnresolvableOpenspec(t *testing.T) {
	h := newHub(t)
	if err := h.CloseTask(testProject, "os-abc123"); err == nil {
		t.Fatalf("closing an unresolvable openspec row should error")
	}
}

// TestApprovePR covers the human approve path: an open PR reaches "approved"
// without a reviewer agent, approving a non-open PR is refused (the open-only
// guard, mirroring the reviewer approve), and an unknown PR errors.
func TestApprovePR(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutPR(store.PR{ID: "pr-td-1", Task: "td-1", Agent: "brokkr", Branch: "td-1", Base: "main"}); err != nil {
		t.Fatalf("put pr: %v", err)
	}

	// Human approve moves an open PR to approved, no reviewer agent involved.
	if err := h.ApprovePR(testProject, "pr-td-1"); err != nil {
		t.Fatalf("approve open PR: %v", err)
	}
	pr, ok, err := ps.GetPR("pr-td-1")
	if err != nil || !ok {
		t.Fatalf("get pr: ok=%v err=%v", ok, err)
	}
	if pr.Status != "approved" {
		t.Fatalf("status = %q, want approved", pr.Status)
	}

	// Open-only guard: an already-approved (non-open) PR cannot be re-approved.
	if err := h.ApprovePR(testProject, "pr-td-1"); err == nil {
		t.Fatalf("approving a non-open PR should be refused")
	}

	// Unknown PR errors.
	if err := h.ApprovePR(testProject, "pr-nope"); err == nil {
		t.Fatalf("approving an unknown PR should error")
	}
}
