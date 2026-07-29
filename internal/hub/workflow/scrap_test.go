package workflow

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/store"
)

// stubDeps is a no-op workflow.Deps that records the agents it interrupts/injects —
// enough to drive ScrapPR without a real hub. (First workflow Engine test harness;
// extend as more Engine methods get covered.)
type stubDeps struct {
	root        string
	alive       bool
	interrupted []string
	injected    []string
}

func (d *stubDeps) ProjectRoot(string) string                   { return d.root }
func (d *stubDeps) ProjectConfig(string) (config.Config, error) { return config.Config{}, nil }
func (d *stubDeps) ArchitectureDoc(string) string               { return "" }
func (d *stubDeps) Container(_, name string) string             { return name }
func (d *stubDeps) Notify()                                     {}
func (d *stubDeps) InjectWhenReady(_, name, _ string) error {
	d.injected = append(d.injected, name)
	return nil
}
func (d *stubDeps) Interrupt(_, name string) error {
	d.interrupted = append(d.interrupted, name)
	return nil
}
func (d *stubDeps) AgentAlive(_, _ string) bool              { return d.alive }
func (d *stubDeps) SessionAlive(_, _ string) bool            { return false }
func (d *stubDeps) TaskComments(_, _ string) []store.Comment { return nil }
func (d *stubDeps) Subscribe() (chan struct{}, func())       { return make(chan struct{}), func() {} }
func (d *stubDeps) KnownProjects() []store.Project           { return nil }
func (d *stubDeps) BrokkrBin() (string, error)               { return "", nil }

// TestScrapPRStopsReviewer: scrapping a PR under review flips it to "scrapped",
// interrupts the reviewer and closes its open review record, so the reviewer no
// longer shows as reviewing a PR whose branch is gone. (ProjectRoot points at a
// non-repo, so the branch delete no-ops-with-a-log — the status flip must still land.)
func TestScrapPRStopsReviewer(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "td-1", Agent: "wrk", Branch: "td-1", Status: "submitted"}); err != nil {
		t.Fatal(err)
	}
	rid, err := ps.AddReview("pr-1", "check it")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.AssignReview(rid, "rev"); err != nil {
		t.Fatal(err)
	}
	if got, _ := ps.ReviewingPR("rev"); got != "pr-1" {
		t.Fatalf("precondition: reviewer should be reviewing pr-1, got %q", got)
	}

	e := New(st, &stubDeps{root: t.TempDir(), alive: true})
	if err := e.ScrapPR("repo", "pr-1"); err != nil {
		t.Fatalf("ScrapPR: %v", err)
	}

	if pr, _, _ := ps.GetPR("pr-1"); pr.Status != "scrapped" {
		t.Fatalf("PR status = %q, want scrapped", pr.Status)
	}
	if got, _ := ps.ReviewingPR("rev"); got != "" {
		t.Fatalf("reviewer should no longer be reviewing (verdict recorded), got %q", got)
	}
}

// TestDiscardPRReleasesItsAuthor: scrapping a PR on its own has no paired task close to free
// the author, so DiscardPR must — otherwise a planner whose proposal the user discards waits in
// "submitted" for a verdict on a PR that no longer exists.
func TestDiscardPRReleasesItsAuthor(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "galar", Role: "planner", Workspace: "."}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-os-new", Task: "os-new", Agent: "galar", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "galar", Task: "os-new", Phase: "submitted"}); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{root: root, alive: true}
	e := New(st, deps)

	if err := e.DiscardPR("proj", "pr-os-new"); err != nil {
		t.Fatalf("DiscardPR: %v", err)
	}
	if pr, _, _ := ps.GetPR("pr-os-new"); pr.Status != "scrapped" {
		t.Errorf("PR status = %q, want scrapped", pr.Status)
	}
	if got, _ := ps.GetState("galar"); got.Phase != "idle" {
		t.Errorf("author phase = %q, want idle — it must not wait on a PR that is gone", got.Phase)
	}
	if len(deps.injected) == 0 {
		t.Error("the author must be told its PR was scrapped")
	}
}

// TestDiscardPRLeavesAnUninvolvedAgentAlone: an author that has moved on must not be
// interrupted for a verdict it is not expecting.
func TestDiscardPRLeavesAnUninvolvedAgentAlone(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker", Workspace: "."}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-td-1", Task: "td-1", Agent: "eitri", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	// Already working something else — not waiting on this PR.
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: "td-9", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{root: root, alive: true}
	e := New(st, deps)

	if err := e.DiscardPR("proj", "pr-td-1"); err != nil {
		t.Fatalf("DiscardPR: %v", err)
	}
	if got, _ := ps.GetState("eitri"); got.Phase != "working" {
		t.Errorf("phase = %q, want working left untouched", got.Phase)
	}
	if len(deps.interrupted) != 0 {
		t.Errorf("an agent not waiting on the PR must not be interrupted, got %v", deps.interrupted)
	}
}
