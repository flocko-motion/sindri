package pr

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// testProject is the one repo these fixtures register.
const testProject = "repo"

// newAct is a pull request's acting half over a real store. The store is real on purpose: what a
// listing SHOWS is a question about rows, and a fake one would assert nothing about that.
func newAct(t *testing.T) (*Act, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject(testProject, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	return New(&core.Core{Store: st}), st.For(testProject)
}

// listAs runs `prs` as an agent and returns what it wrote. Direct rather than through the hub's exec
// surface: which verbs a state OFFERS is the registry's business and is tested there, so a listing
// test that went the long way would be asserting somebody else's rule.
func listAs(t *testing.T, a *Act, ps *store.ProjectStore, agent string, args ...string) (string, int) {
	t.Helper()
	ag, ok, err := ps.GetAgent(agent)
	if err != nil || !ok {
		t.Fatalf("no agent %q on the roster", agent)
	}
	var out bytes.Buffer
	code, err := a.CmdListPRs(registry.Caller{Project: testProject, Agent: agent, Role: ag.Role}, args, &out)
	if err != nil {
		t.Fatalf("prs %v: %v", args, err)
	}
	return out.String(), code
}
