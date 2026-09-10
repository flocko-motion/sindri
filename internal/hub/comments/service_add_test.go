package comments

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// stubDeps is the seam back to the hub: a root to resolve and a notify to count.
type stubDeps struct {
	root     string
	notified int
}

func (d *stubDeps) ProjectRoot(string) string { return d.root }
func (d *stubDeps) Notify()                   { d.notified++ }

func newService(t *testing.T) (*Service, *store.ProjectStore, *stubDeps) {
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
	return New(st, d), st.For("proj"), d
}

// TestCommentOnAnOwnedTaskIsKeptHere: nothing upstream holds the thread for a task sindri owns, so
// this store is where the comment lives rather than a cache of somewhere else.
func TestCommentOnAnOwnedTaskIsKeptHere(t *testing.T) {
	s, ps, d := newService(t)
	if err := s.Add("proj", "td-abc123", "user", "the thing to remember"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, err := ps.Comments("td-abc123")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want one comment, got %d", len(got))
	}
	if got[0].Body != "the thing to remember" {
		t.Errorf("body = %q", got[0].Body)
	}
	if got[0].Source != LocalSource {
		t.Errorf("source = %q, want %q — the hub owns this thread", got[0].Source, LocalSource)
	}
	if got[0].CreatedAt == "" || got[0].SourceRef == "" {
		t.Errorf("a comment needs a timestamp and an id: %+v", got[0])
	}
	if got[0].Author != "user" {
		t.Errorf("author = %q, want %q", got[0].Author, "user")
	}
	if d.notified == 0 {
		t.Error("the board should be woken so the comment appears")
	}
}

// TestAnAgentsCommentIsAttributedToTheAgent: the author recorded is whoever actually called Add,
// not a hardcoded stand-in — the bug that made every local comment read "user" regardless of who
// (human or agent) wrote it.
func TestAnAgentsCommentIsAttributedToTheAgent(t *testing.T) {
	s, ps, _ := newService(t)
	if err := s.Add("proj", "td-abc123", "dvalin", "found a blocker"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, _ := ps.Comments("td-abc123")
	if len(got) != 1 || got[0].Author != "dvalin" {
		t.Fatalf("comments = %+v, want one authored by dvalin", got)
	}
}

// TestCommentsAccumulate: a thread is a sequence, so a second comment joins the first rather than
// replacing it — the mistake a reconciling write would make here.
func TestCommentsAccumulate(t *testing.T) {
	s, ps, _ := newService(t)
	for _, body := range []string{"first", "second", "third"} {
		if err := s.Add("proj", "os-a1b2c3", "user", body); err != nil {
			t.Fatalf("Add %q: %v", body, err)
		}
	}
	got, _ := ps.Comments("os-a1b2c3")
	if len(got) != 3 {
		t.Fatalf("want 3 comments, got %d", len(got))
	}
}

// TestAnEmptyCommentIsRefused, so a stray keystroke cannot post nothing.
func TestAnEmptyCommentIsRefused(t *testing.T) {
	s, ps, _ := newService(t)
	if err := s.Add("proj", "td-abc123", "user", "   "); err == nil {
		t.Error("an empty comment must be refused")
	}
	if got, _ := ps.Comments("td-abc123"); len(got) != 0 {
		t.Errorf("nothing should have been recorded, got %d", len(got))
	}
}

// TestOurCommentsSurviveAForeignSync is the property the two-way split rests on: a GitHub re-fetch
// reconciles ITS thread, and a comment the hub owns is not part of that set.
func TestOurCommentsSurviveAForeignSync(t *testing.T) {
	_, ps, _ := newService(t)
	if err := ps.AddComment("gh-42", store.Comment{
		Source: LocalSource, SourceRef: "abc", Author: "user", Body: "kept here", CreatedAt: "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	// The GitHub thread is re-read and replaced, as a sync does.
	if err := ps.ReplaceComments("gh-42", "github", []store.Comment{
		{Source: "github", SourceRef: "u1", Author: "octocat", Body: "from the issue", CreatedAt: "2026-01-02T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := ps.Comments("gh-42")
	if len(got) != 2 {
		t.Fatalf("want both comments, got %d: %+v", len(got), got)
	}
	var sources []string
	for _, c := range got {
		sources = append(sources, c.Source)
	}
	joined := strings.Join(sources, ",")
	if !strings.Contains(joined, LocalSource) {
		t.Errorf("a GitHub sync must not drop the hub's own comment (sources: %s)", joined)
	}
}
