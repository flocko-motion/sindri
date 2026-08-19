package workflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestExplainNextNamesEveryReason walks the backlog shapes that have each cost an afternoon this
// week: a gated proposal, an unrated task, a package's child, and the spent parent whose children
// have all closed — which td-8c0187 made claimable again, so its holder finishes it on the branch
// the work is already on.
func TestExplainNextNamesEveryReason(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	for _, x := range []store.Task{
		{ID: "td-ready", Title: "claimable", Status: "open", Priority: "P1"},
		{ID: "td-gated", Title: "a proposal", Status: "open", Priority: "P1"},
		{ID: "td-bare", Title: "no priority", Status: "open"},
		{ID: "td-pkg", Title: "a package", Status: "open", Priority: "P2"},
		{ID: "td-kid", Title: "inside it", Status: "open", ParentID: "td-pkg"},
		{ID: "os-spent", Title: "finished, never closed", Status: "open", Priority: "P1"},
		{ID: "td-done", Title: "its only child", Status: "closed", ParentID: "os-spent"},
	} {
		if err := ps.UpsertTask(x); err != nil {
			t.Fatal(err)
		}
	}
	if err := ps.SetApproval("td-gated", "pending", ""); err != nil {
		t.Fatal(err)
	}

	e := New(st, &stubDeps{root: t.TempDir()})
	x, err := e.ExplainNext("repo", "", "")
	if err != nil {
		t.Fatalf("ExplainNext: %v", err)
	}
	got := map[string]api.Claimability{}
	for _, r := range x.Tasks {
		got[r.ID] = r.Why
	}
	for id, want := range map[string]api.Claimability{
		"td-ready": api.ClaimableTask,
		"td-gated": api.AwaitingApproval,
		"td-bare":  api.Unrated,
		"td-pkg":   api.ClaimablePackage,
		"td-kid":   api.InsideAPackage,
		// Spent, not stranded: it had a child, it has not landed, so it is offered and its holder
		// submits the branch. Nothing else would ever release it.
		"os-spent": api.ClaimablePackage,
	} {
		if got[id] != want {
			t.Errorf("%s: %q, want %q", id, got[id], want)
		}
	}
	if _, listed := got["td-done"]; listed {
		t.Error("a closed task is not waiting for anything and should not be listed")
	}
	// Packages and leaves are ranked together (-> nextUp), so the P2 package loses to both P1s, and
	// the tie between the P1 package and the P1 leaf falls to the id.
	if x.Pick == nil || x.Pick.ID != "os-spent" {
		t.Errorf("pick = %+v, want os-spent (P1 beats the P2 package, and it sorts before td-ready)", x.Pick)
	}
	// Nothing may be stranded: that category is a bug detector, not a state the backlog can be in.
	for _, r := range x.Tasks {
		if r.Why == api.Stranded {
			t.Errorf("%s is offered by no pool — a hole, not an explanation", r.ID)
		}
	}
	t.Logf("\n%s", strings.TrimRight(formatForTest(x), "\n"))
}

// TestExplainNextAnswersForAnAgent: an agent's own state can rule out the whole backlog, and saying
// which is the point — a retired or busy worker is not the backlog being empty.
func TestExplainNextAnswersForAnAgent(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.UpsertTask(store.Task{ID: "td-ready", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutAgent(store.Agent{Name: "bombur", Role: "worker", Retired: true}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "bombur", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	e := New(st, &stubDeps{root: t.TempDir()})

	x, err := e.ExplainNext("repo", "bombur", "")
	if err != nil {
		t.Fatalf("ExplainNext: %v", err)
	}
	if !strings.Contains(x.AgentNote, "retired") {
		t.Errorf("AgentNote = %q, want it to name the retirement", x.AgentNote)
	}
	if x.Pick != nil {
		t.Errorf("a retired agent is handed nothing, got %+v", x.Pick)
	}
	// The claimable task is still reported as claimable: the backlog is fine, this agent is not.
	for _, r := range x.Tasks {
		if r.ID == "td-ready" && !r.Claimable() {
			t.Errorf("td-ready = %q, want it still claimable — the agent is what is blocked", r.Why)
		}
	}
}

// TestExplainNextRulesOutARetiredOrClearArmedAgent closes the gap nudgeIdleWorkers's herd fix
// (sd-4589ef) would otherwise inherit: agentBlocked used to check only a held task, so a
// human-retired or clear-armed agent still showed a Pick as though it were free.
func TestExplainNextRulesOutARetiredOrClearArmedAgent(t *testing.T) {
	for _, mutate := range []struct {
		name string
		do   func(a *store.Agent)
		want string
	}{
		{"retired", func(a *store.Agent) { a.Retired = true }, "retired"},
		{"clear-armed", func(a *store.Agent) { a.ClearArmed = true }, "clear"},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { st.Close() })
			ps := st.For("repo")
			if err := ps.UpsertTask(store.Task{ID: "td-ready", Status: "open", Priority: "P1"}); err != nil {
				t.Fatal(err)
			}
			a := store.Agent{Name: "bombur", Role: "worker"}
			mutate.do(&a)
			if err := ps.PutAgent(a); err != nil {
				t.Fatal(err)
			}
			if err := ps.SetState(store.AgentState{Agent: "bombur", Phase: "idle"}); err != nil {
				t.Fatal(err)
			}
			e := New(st, &stubDeps{root: t.TempDir()})

			x, err := e.ExplainNext("repo", "bombur", "")
			if err != nil {
				t.Fatalf("ExplainNext: %v", err)
			}
			if !strings.Contains(x.AgentNote, mutate.want) {
				t.Errorf("AgentNote = %q, want it to name why (%q)", x.AgentNote, mutate.want)
			}
			if x.Pick != nil {
				t.Errorf("a %s agent is handed nothing, got %+v", mutate.name, x.Pick)
			}
		})
	}
}

// formatForTest renders the explanation the way a front-end would, so the test log shows what a user
// would actually read rather than a struct dump.
func formatForTest(x api.NextExplain) string {
	var b strings.Builder
	if x.Pick != nil {
		b.WriteString("next: " + x.Pick.ID + "  (" + string(x.Pick.Why) + ")\n")
	}
	for _, r := range x.Tasks {
		b.WriteString(r.ID + "  " + string(r.Why) + "\n")
	}
	return b.String()
}
