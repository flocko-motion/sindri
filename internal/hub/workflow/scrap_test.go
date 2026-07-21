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
