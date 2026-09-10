package comments

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// stubSource is a minimal tasks.Source that claims every id it's asked about and records the body
// it was handed — enough to assert what Add sends upstream without a real tracker call.
type stubSource struct {
	posted map[string]string // id -> last body AddComment received
}

func (*stubSource) Name() string                                          { return "stub" }
func (*stubSource) Enabled(string) bool                                   { return true }
func (*stubSource) ToolMissing(string) bool                               { return false }
func (*stubSource) Tasks(string, bool) ([]task.Task, error)               { return nil, nil }
func (*stubSource) OnMerged(string, string, string) error                 { return nil }
func (*stubSource) Finish(string, string, bool) (bool, error)             { return false, nil }
func (*stubSource) Comments(string, string) ([]task.Comment, bool, error) { return nil, false, nil }

func (s *stubSource) AddComment(_, id, body string) (bool, error) {
	if s.posted == nil {
		s.posted = map[string]string{}
	}
	s.posted[id] = body
	return true, nil
}

// newServiceWithSource is newService, but wired with a tracker stub instead of none — for asserting
// what actually gets posted upstream.
func newServiceWithSource(t *testing.T, src *stubSource) (*Service, *stubDeps) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	d := &stubDeps{root: root}
	return New(st, d, src), d
}

// TestAnAgentsUpstreamCommentNamesItself is the bug a review caught: the tracker's own AddComment
// carries no author, so it posts as whichever identity `gh` authenticates with — the human's,
// unless the agent's name is put into the body itself. Both the issue's own readers and sindri's
// re-synced local copy (which loses the Author field to the tracker's own account on the next sync)
// depend on that prefix to know who actually wrote it.
func TestAnAgentsUpstreamCommentNamesItself(t *testing.T) {
	stub := &stubSource{}
	s, _ := newServiceWithSource(t, stub)
	if err := s.Add("proj", "gh-42", "dvalin", "found a blocker"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	posted := stub.posted["gh-42"]
	if !strings.Contains(posted, "dvalin") {
		t.Errorf("posted body = %q, want it to name dvalin", posted)
	}
	if !strings.Contains(posted, "found a blocker") {
		t.Errorf("posted body = %q, want the original text preserved", posted)
	}
}

// TestAHumansUpstreamCommentIsUnchanged: there is no agent identity to lose, so nothing is added.
func TestAHumansUpstreamCommentIsUnchanged(t *testing.T) {
	stub := &stubSource{}
	s, _ := newServiceWithSource(t, stub)
	if err := s.Add("proj", "gh-42", api.SenderUser, "looks good to me"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if posted := stub.posted["gh-42"]; posted != "looks good to me" {
		t.Errorf("posted body = %q, want it unchanged", posted)
	}
}
