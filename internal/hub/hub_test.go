package hub

import (
	"context"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/workflow"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

const testProject = "proj"

// newHub opens a global hub rooted at a temp state dir (via SINDRI_HOME), so tests never touch the
// real ~/.local/state/sindri. toolskew's two lookups default to a no-op here (-> noHostVersions,
// noPodManifest): the real ones read a developer's actual image cache and shell out, and only
// toolskew's own tests (internal/hub/toolskew_test.go) need anything else.
func newHub(t *testing.T) *Hub {
	t.Helper()
	t.Setenv("SINDRI_HOME", t.TempDir())
	h, err := open(t.Context(), noHostVersions, noPodManifest)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

func noHostVersions(context.Context) map[string]string { return nil }
func noPodManifest() (map[string]string, error)        { return nil, nil }

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
	if _, err := h.agents.NewAgent(testProject, "Brokkr", "worker", ""); err == nil {
		t.Fatalf("uppercase name should be rejected")
	}
	if _, err := h.agents.NewAgent(testProject, "brokkr", "boss", ""); err == nil {
		t.Fatalf("bad role should be rejected")
	}
	if _, err := h.agents.NewAgent(testProject, "brokkr", "worker", ""); err != nil {
		t.Fatalf("valid agent: %v", err)
	}
	if _, err := h.agents.NewAgent(testProject, "brokkr", "worker", ""); err == nil {
		t.Fatalf("duplicate agent should be rejected")
	}
}

// TestGlobalProjectAcceptsOnlyAReviewer: _global holds no repo, so nothing a worker, planner or
// coauthor carries across its work — a branch, a standing conversation, the user's own seat —
// exists there. A reviewer, which carries nothing between reviews, is the one role that fits.
func TestGlobalProjectAcceptsOnlyAReviewer(t *testing.T) {
	h := newHub(t)
	for _, role := range []string{"worker", "planner", "coauthor"} {
		if _, err := h.agents.NewAgent(workflow.GlobalProject, "x-"+role, role, ""); err == nil {
			t.Errorf("a %s should be refused in %s", role, workflow.GlobalProject)
		}
	}
	if _, err := h.agents.NewAgent(workflow.GlobalProject, "ori", "reviewer", ""); err != nil {
		t.Errorf("a reviewer should be accepted in %s: %v", workflow.GlobalProject, err)
	}
}

func TestNewAgentAutoName(t *testing.T) {
	h := newHub(t)
	n1, err := h.agents.NewAgent(testProject, "", "worker", "")
	if err != nil {
		t.Fatal(err)
	}
	// A different project must still get a globally-unique name (not reuse n1).
	n2, err := h.agents.NewAgent("other", "", "worker", "")
	if err != nil {
		t.Fatal(err)
	}
	if n1 == "" || n2 == "" {
		t.Fatalf("auto-name should be non-empty: %q, %q", n1, n2)
	}
	// The dwarf pool itself is unit-tested in internal/hub/agent; here we assert the
	// hub-level behaviour: auto-names are globally unique across projects.
	if n1 == n2 {
		t.Fatalf("auto-names must be globally unique across projects, got %q twice", n1)
	}
	if n1 == "sindri" || n2 == "sindri" || n1 == "brokkr" || n2 == "brokkr" {
		t.Fatalf("must never hand out a binary name")
	}
}

func TestNewAgentNameGloballyUnique(t *testing.T) {
	h := newHub(t)
	if _, err := h.agents.NewAgent("repoA", "eitri", "worker", ""); err != nil {
		t.Fatal(err)
	}
	// The same name in a DIFFERENT repo is refused — names are unique machine-wide.
	if _, err := h.agents.NewAgent("repoB", "eitri", "worker", ""); err == nil {
		t.Fatalf("same name in another repo should be rejected (global uniqueness)")
	}
}

func TestNewAgentRecordsIdentityAndLog(t *testing.T) {
	h := newHub(t)
	if _, err := h.agents.NewAgent(testProject, "dvalin", "reviewer", ""); err != nil {
		t.Fatal(err)
	}
	// Observe before asserting. The agent is registered after the watchdog seeded, so until a sweep
	// looks at it its status is "unknown" — correct, and a race to assert around: whether this read
	// caught the settled value depended on the tick landing first. Probes included, since the
	// listing alone leaves an existing pod's session unread.
	h.watch.sweep(true)
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

// TestHubNeverSeedsArchitectureDoc: registering a repo must not write an
// ARCHITECTURE.md into it. The hub used to seed a placeholder, which littered every repo
// it touched; an architecture doc is the project's to create, and the hub only advises.
func TestHubNeverSeedsArchitectureDoc(t *testing.T) {
	h := newHub(t)
	root := t.TempDir()
	_ = h.repo(root)

	if _, err := os.Stat(filepath.Join(root, "ARCHITECTURE.md")); !os.IsNotExist(err) {
		t.Errorf("registering a repo must not create ARCHITECTURE.md (stat err: %v)", err)
	}
	// The one file the hub does maintain, for its own artifacts, is still written.
	if _, err := os.Stat(filepath.Join(root, ".gitignore")); err != nil {
		t.Errorf(".gitignore should still be maintained: %v", err)
	}
}

// TestStartupAdvice: instead of seeding, the hub recommends once at startup — and says
// nothing about a repo that's in good shape.
func TestStartupAdvice(t *testing.T) {
	h := newHub(t)

	// Unconfigured, no doc → recommend, naming the key and the file to put it in.
	repo := t.TempDir()
	_ = h.repo(repo)
	advice := strings.Join(h.StartupAdvice(), "\n")
	if !strings.Contains(advice, "no architecture doc") || !strings.Contains(advice, "architecture: <path>") {
		t.Errorf("expected a recommendation naming the config key, got:\n%s", advice)
	}
	if !strings.Contains(advice, filepath.Base(repo)) {
		t.Errorf("advice should name the repo, got:\n%s", advice)
	}

	// Present at the default path → silent.
	if err := os.WriteFile(filepath.Join(repo, "ARCHITECTURE.md"), []byte("# arch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := h.StartupAdvice(); len(got) != 0 {
		t.Errorf("a repo with a readable doc needs no advice, got %v", got)
	}

	// Configured elsewhere and present → also silent (no nagging about the default name).
	if err := os.MkdirAll(filepath.Join(repo, ".sindri", "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".sindri", "docs", "ARCH.md"), []byte("# arch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".sindri", "config.yaml"), []byte("architecture: .sindri/docs/ARCH.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repo, "ARCHITECTURE.md")); err != nil {
		t.Fatal(err)
	}
	if got := h.StartupAdvice(); len(got) != 0 {
		t.Errorf("a repo that configured its own doc needs no advice, got %v", got)
	}

	// A config that won't load is surfaced here rather than days later, at first use.
	// A configured-but-absent architecture path lands in this bucket: validate rejects it.
	if err := os.WriteFile(filepath.Join(repo, ".sindri", "config.yaml"), []byte("architecture: docs/GONE.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	advice = strings.Join(h.StartupAdvice(), "\n")
	if !strings.Contains(advice, "GONE.md") {
		t.Errorf("a broken config should be reported and name the path, got:\n%s", advice)
	}
}

// TestReviewInstructionsCarryArchitecture: both review-instruction paths (the no-arg
// `sindri` directive = workflow.DirReview, and the injected = workflow.MsgReview) always tell the
// reviewer to read the repo's ARCHITECTURE.md.
func TestReviewInstructionsCarryArchitecture(t *testing.T) {
	if !strings.Contains(workflow.DirReview("pr-1", "td-1", "a task title", "dwalin", "ARCHITECTURE.md"), "ARCHITECTURE.md") {
		t.Errorf("workflow.DirReview must tell the reviewer to read the architecture doc")
	}
	if !strings.Contains(workflow.MsgReview("pr-1", "req", "br", "base", "ARCHITECTURE.md", true), "ARCHITECTURE.md") {
		t.Errorf("workflow.MsgReview must tell the reviewer to read the architecture doc")
	}
}

func TestTellUnknownAgent(t *testing.T) {
	h := newHub(t)
	if err := h.agents.Tell(t.Context(), testProject, "ghost", "hi", "user", api.SignedOutRefuse); err == nil {
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
// TestImportCarriesTheTdBacklogOnce: a repo arriving with a td store keeps its tasks, and the import
// runs once. Repeating it would resurrect everything closed since, because td still holds the state
// it had at migration. What td gains afterwards is deliberately ignored — sindri owns the tasks now.
func TestImportCarriesTheTdBacklogOnce(t *testing.T) {
	if _, err := exec.LookPath("td"); err != nil {
		t.Skip("td CLI not installed")
	}
	h := newHub(t)
	root := t.TempDir()
	if out, err := exec.Command("td", "-w", root, "init").CombinedOutput(); err != nil {
		t.Fatalf("td init: %s", out)
	}
	tdCreate(t, root, "-t", "feature", "First task in the backlog")

	ps := h.repo(root)
	tag := RepoTag(root)
	if err := h.Refresh(tag); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	after, _ := ps.AllTasks()
	if !hasTaskTitled(after, "First task in the backlog") {
		t.Fatalf("the existing td backlog was not imported: %+v", after)
	}
	owned, err := ps.OwnedTasks()
	if err != nil || len(owned) != 1 {
		t.Fatalf("the import must land in the owned table: %d task(s), err %v", len(owned), err)
	}

	// Closing it and re-syncing must not bring it back: the import is spent.
	if err := h.wf.CloseTask(tag, owned[0].ID); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := h.Refresh(tag); err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	got, _, _ := ps.OwnedTask(owned[0].ID)
	if got.Status != "closed" {
		t.Errorf("status %q after a re-sync, want closed — a second import would undo the close", got.Status)
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

// TestClosingAnAlreadyEndedOpenspecChangeSucceeds: ending the change is what closing is FOR, so a
// change that is no longer active satisfies the postcondition. A worker archives the change itself
// as part of the work — that is the change — and its checkpoint then failed on the hub and
// escalated it for having finished. A SCRAP still errors: there the caller means to destroy
// something it expects to find. (A real os- close archives the change; that needs the openspec CLI
// and a change, so it is exercised end-to-end, not here.)
func TestClosingAnAlreadyEndedOpenspecChangeSucceeds(t *testing.T) {
	h := newHub(t)
	if err := h.wf.CloseTask(testProject, "os-abc123"); err != nil {
		t.Fatalf("an already-ended change is the postcondition, not a failure: %v", err)
	}
	if err := h.wf.ScrapTask(testProject, "os-abc123", false, false); err == nil {
		t.Error("a scrap means to destroy something it expects to find, so it must still say it is missing")
	}
}

// TestCloseFreesWorkingAgent: closing a task an agent is working on is ALLOWED (not
// refused) — the task closes and the agent is freed (idle, no task) so it picks up
// new work rather than grinding on a cancelled task. td-gated (needs a real store).
func TestCloseFreesWorkingAgent(t *testing.T) {
	if _, err := exec.LookPath("td"); err != nil {
		t.Skip("td CLI not installed")
	}
	h := newHub(t)
	root := t.TempDir()
	if out, err := exec.Command("td", "-w", root, "init").CombinedOutput(); err != nil {
		t.Fatalf("td init: %s", out)
	}
	h.repo(root)
	tag := RepoTag(root)
	id, err := h.wf.CreateTask(tag, api.TaskSpec{Title: "implement the widget feature", Type: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.agents.NewAgent(tag, "eitri", "worker", ""); err != nil {
		t.Fatal(err)
	}
	ps := h.store.For(tag)
	_ = ps.SetState(store.AgentState{Agent: "eitri", Task: id, Branch: id, Phase: "working"}, store.ReasonClaimed, "test setup")

	if err := h.wf.CloseTask(tag, id); err != nil { // must NOT refuse just because eitri holds it
		t.Fatalf("closing a held task should be allowed: %v", err)
	}
	if st, _ := ps.GetState("eitri"); st.Task != "" || st.Phase == "working" {
		t.Fatalf("agent should be freed after its task was closed, got phase=%q task=%q", st.Phase, st.Task)
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
	if err := h.wf.ApprovePR(testProject, "pr-td-1"); err != nil {
		t.Fatalf("approve open PR: %v", err)
	}
	pr, ok, err := ps.GetPR("pr-td-1")
	if err != nil || !ok {
		t.Fatalf("get pr: ok=%v err=%v", ok, err)
	}
	if pr.Status != "approved" {
		t.Fatalf("status = %q, want approved", pr.Status)
	}

	// Approvals accumulate: an already-approved PR takes a second approval as another badge,
	// rather than the first verdict locking out any that follow.
	if err := h.wf.ApprovePR(testProject, "pr-td-1"); err != nil {
		t.Fatalf("re-approving an approved PR should accumulate a badge: %v", err)
	}
	if revs, rerr := ps.Reviews("pr-td-1"); rerr != nil || api.ApprovalCount(revs) != 2 {
		t.Fatalf("reviews=%v err=%v, want 2 approval badges", revs, rerr)
	}

	// A rejection still dominates: once rejected, only a renewed submission — not a fresh
	// approval — clears it.
	pr, _, _ = ps.GetPR("pr-td-1")
	pr.Status = "rejected"
	if err := ps.PutPR(pr); err != nil {
		t.Fatalf("put pr: %v", err)
	}
	if err := h.wf.ApprovePR(testProject, "pr-td-1"); err == nil {
		t.Fatalf("approving a rejected PR should be refused")
	}

	// Unknown PR errors.
	if err := h.wf.ApprovePR(testProject, "pr-nope"); err == nil {
		t.Fatalf("approving an unknown PR should error")
	}
}

// TestReviewerReadsButCannotAct is the case for letting a reviewer read the backlog: reading is
// inert. The authority lives in the registry's role lists rather than in the handlers, so that is
// what this asks — a reviewer gains the read verb and none that change the work it judges.
func TestReviewerReadsButCannotAct(t *testing.T) {
	h := newHub(t)
	reg := h.registry()
	available := func(role string) map[string]bool {
		out := map[string]bool{}
		for _, c := range reg.Available(registry.Caller{Project: testProject, Agent: "rune", Role: role}) {
			out[c.Name] = true
		}
		return out
	}
	rev := available("reviewer")
	if !rev["task"] {
		t.Error("a reviewer cannot read the task it is reviewing")
	}
	// Reading is the whole grant. Anything that proposes, edits or re-orders the work under review
	// would make the reviewer a party to what it is judging.
	for _, verb := range []string{"create-task", "edit-task", "prioritise-task", "reopen-task", "next", "submit"} {
		if rev[verb] {
			t.Errorf("a reviewer must not have %q — reading grants no authority", verb)
		}
	}
	// And the grant is additive: the roles that already read still do.
	for _, role := range []string{"planner", "worker", "coauthor"} {
		if !available(role)["task"] {
			t.Errorf("%s lost the task verb", role)
		}
	}
}

// TestPlannerGainsApproveButNotReject: a planner may add its optional, advisory badge (-> pr.go
// CmdApprove's role branch), but never a reject — the planner grant is a second opinion beside a
// verdict, not a verdict of its own. (A coauthor's IS a verdict: -> the test below.)
func TestPlannerGainsApproveButNotReject(t *testing.T) {
	h := newHub(t)
	reg := h.registry()
	available := func(role string) map[string]bool {
		out := map[string]bool{}
		for _, c := range reg.Available(registry.Caller{Project: testProject, Agent: "rune", Role: role}) {
			out[c.Name] = true
		}
		return out
	}
	if !available("planner")["approve"] {
		t.Error("a planner must be able to add its optional approval badge")
	}
	if available("planner")["reject"] {
		t.Error("a planner must not be able to reject — that stays the reviewer's alone")
	}
	if available("worker")["approve"] {
		t.Error("a worker must not gain approve — it would be ruling on the work it builds")
	}
}

// TestCoauthorGainsAuthorshipAndVerdicts (sd-44550c): the strongest role, driven directly by the
// user, gains the verbs it could only read around before — it shapes the backlog it already reads,
// and records what it concluded about a PR it can already diff and lint. It gains no QUEUE with
// them: nothing hands a coauthor work, which is what keeps it freestyle and non-blocking.
func TestCoauthorGainsAuthorshipAndVerdicts(t *testing.T) {
	h := newHub(t)
	reg := h.registry()
	available := func(role string) map[string]bool {
		out := map[string]bool{}
		for _, c := range reg.Available(registry.Caller{Project: testProject, Agent: "rune", Role: role}) {
			out[c.Name] = true
		}
		return out
	}
	co := available("coauthor")
	for _, verb := range []string{"create-task", "edit-task", "approve", "reject"} {
		if !co[verb] {
			t.Errorf("a coauthor must have %q — it reads the backlog and the PRs already", verb)
		}
	}
	// Its own second workspace (sd-a6e45e), and nobody else's: every other role already has a
	// worktree of its own to check work out into.
	if !co["scratch"] {
		t.Error("a coauthor must have `scratch` — it is the only way it can read another agent's code")
	}
	for _, role := range []string{"worker", "reviewer", "planner"} {
		if available(role)["scratch"] {
			t.Errorf("%s must not have `scratch` — it works in a worktree of its own already", role)
		}
	}
	// The verbs, not the queue: these are how work is HANDED to an agent, and a coauthor takes none.
	for _, verb := range []string{"next", "submit", "checkpoint"} {
		if co[verb] {
			t.Errorf("a coauthor must not have %q — its work comes from the user, never a queue", verb)
		}
	}
	// The grant is the coauthor's, not everyone's: a worker still cannot author or rule on tasks.
	for _, verb := range []string{"create-task", "edit-task", "approve", "reject"} {
		if available("worker")[verb] {
			t.Errorf("a worker must not gain %q with the coauthor grant", verb)
		}
	}
}

// TestRunServiceReachesEveryRoleThatCanUseIt (sd-68f8e7): worker, reviewer and coauthor can
// schedule a run; a planner cannot — its workspace is read-only, so there is nothing for it to run.
func TestRunServiceReachesEveryRoleThatCanUseIt(t *testing.T) {
	h := newHub(t)
	reg := h.registry()
	available := func(role string) map[string]bool {
		out := map[string]bool{}
		for _, c := range reg.Available(registry.Caller{Project: testProject, Agent: "rune", Role: role}) {
			out[c.Name] = true
		}
		return out
	}
	for _, role := range []string{"worker", "reviewer", "coauthor"} {
		if !available(role)["run"] {
			t.Errorf("%s should be able to queue a run", role)
		}
	}
	if available("planner")["run"] {
		t.Error("a planner's workspace is read-only — it must not gain the run service")
	}
}
