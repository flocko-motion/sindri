package mail

import (
	"bytes"
	"io"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// stubDeps records what the mailbox asked of the hub. NOBODY IS UP by default, which is the shape
// mail exists for: a message that must be read cannot depend on a session being there to take it.
// A test that wants a live pane sets `up`.
type stubDeps struct {
	up        bool
	noWake    bool
	pushFails bool     // up, but the keystrokes do not land — a session that will not take them
	pushed    []string // who a push was typed at
	texts     []string // what each push said
	notices   int
}

func (d *stubDeps) Push(project, name, text string) error {
	if !d.up || d.pushFails {
		return errNoSuchAnywhere(name)
	}
	d.pushed = append(d.pushed, name)
	d.texts = append(d.texts, text)
	return nil
}
func (d *stubDeps) Notify()                             { d.notices++ }
func (d *stubDeps) RepoName(project string) string      { return project }
func (d *stubDeps) Reachable(project, name string) bool { return d.up }

// MayWake defaults to yes — the ordinary case. A test that wants the parked half sets `noWake`.
func (d *stubDeps) MayWake(project, name string) bool { return !d.noWake }

// testProject is the one repo these fixtures register, named as the hub's own tests name theirs.
const testProject = "repo"

// newBox is a mailbox over a real store and a recording hub. The store is real on purpose: the
// mailbox's whole guarantee is what SURVIVES, and a fake one would assert nothing about that.
func newBox(t *testing.T) (*Box, *store.ProjectStore, *stubDeps) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject(testProject, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	d := &stubDeps{}
	return New(st, d), st.For(testProject), d
}

// roleOf is what the fixtures seeded each agent as, so a caller carries the role the scoping rules
// read. Looked up rather than passed: a test naming the agent has already said which role it means.
func roleOf(t *testing.T, ps *store.ProjectStore, agent string) string {
	t.Helper()
	a, ok, err := ps.GetAgent(agent)
	if err != nil || !ok {
		t.Fatalf("no agent %q on the roster", agent)
	}
	return a.Role
}

// verbAs runs one of the mailbox's own verbs as an agent, and returns what it wrote. Direct rather
// than through the hub's exec surface: which verbs a state OFFERS is the registry's business and is
// tested there, so a mailbox test that went the long way would be asserting somebody else's rule.
func verbAs(t *testing.T, b *Box, ps *store.ProjectStore, agent string, args ...string) (string, int) {
	t.Helper()
	c := registry.Caller{Project: ps.Project(), Agent: agent, Role: roleOf(t, ps, agent)}
	var out bytes.Buffer
	run := map[string]func(registry.Caller, []string, io.Writer) (int, error){
		"mail": b.CmdMail, "reply": b.CmdReply, "fyi": b.CmdFyi,
	}[args[0]]
	if run == nil {
		t.Fatalf("%q is not one of the mailbox's verbs", args[0])
	}
	code, err := run(c, args[1:], &out)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String(), code
}

// mailAgent seeds a worker holding a task, the shape every sender delivers to.
func mailAgent(t *testing.T) (*Box, *store.ProjectStore) {
	t.Helper()
	b, ps, _ := newBox(t)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-1", Title: "a task", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-1", Branch: "td-1", Phase: "working"},
		store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	return b, ps
}

// newBox2 is a bare mailbox with no roster, for a case that seeds its own.
func newBox2(t *testing.T) (*Box, *store.ProjectStore) {
	t.Helper()
	b, ps, _ := newBox(t)
	return b, ps
}

// verbIn runs a verb as an agent of a NAMED project — verbAs is fixed to the test project, and the
// point of a bare name is that the two ends of a conversation sit in different repos.
func verbIn(t *testing.T, b *Box, project, agent string, args ...string) (string, int) {
	t.Helper()
	return verbAs(t, b, b.store.For(project), agent, args...)
}
