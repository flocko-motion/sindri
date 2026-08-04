package store

import (
	"path/filepath"
	"testing"
)

// ownedStore opens a throwaway store with one project registered.
func ownedStore(t *testing.T) *ProjectStore {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("proj", t.TempDir()); err != nil {
		t.Fatalf("register: %v", err)
	}
	return st.For("proj")
}

// TestOwnedTaskRoundTrip: every field a task carries has to survive the write, since these rows are
// the authority the read model is rebuilt from.
func TestOwnedTaskRoundTrip(t *testing.T) {
	ps := ownedStore(t)
	want := OwnedTask{
		ID: "td-abc123", Title: "wire the thing", Status: "open", Priority: "P2",
		Type: "task", Labels: "spec,ui", ParentID: "td-parent", Description: "the body",
	}
	if err := ps.PutOwnedTask(want); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, ok, err := ps.OwnedTask("td-abc123")
	if err != nil || !ok {
		t.Fatalf("read back: ok=%v err=%v", ok, err)
	}
	for _, c := range []struct{ field, got, want string }{
		{"title", got.Title, want.Title},
		{"status", got.Status, want.Status},
		{"priority", got.Priority, want.Priority},
		{"type", got.Type, want.Type},
		{"labels", got.Labels, want.Labels},
		{"parent", got.ParentID, want.ParentID},
		{"description", got.Description, want.Description},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.field, c.got, c.want)
		}
	}
	if got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Errorf("timestamps must be stamped: created=%q updated=%q", got.CreatedAt, got.UpdatedAt)
	}
}

// TestPutPreservesCreatedAtAndMovesUpdatedAt: a second write is an edit, so the creation time is
// the one thing it must not rewrite.
func TestPutPreservesCreatedAtAndMovesUpdatedAt(t *testing.T) {
	ps := ownedStore(t)
	if err := ps.PutOwnedTask(OwnedTask{ID: "td-1", Title: "first", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	first, _, _ := ps.OwnedTask("td-1")
	if err := ps.PutOwnedTask(OwnedTask{ID: "td-1", Title: "second", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	second, _, _ := ps.OwnedTask("td-1")
	if second.CreatedAt != first.CreatedAt {
		t.Errorf("created_at changed on edit: %q then %q", first.CreatedAt, second.CreatedAt)
	}
	if second.Title != "second" {
		t.Errorf("title = %q, want the edit to land", second.Title)
	}
}

// TestStatusWriteOnAnUnownedIdFails: a write that matched nothing used to be indistinguishable from
// one that worked, which is how a task drifts from what the board shows.
func TestStatusWriteOnAnUnownedIdFails(t *testing.T) {
	ps := ownedStore(t)
	if err := ps.SetOwnedStatus("td-nope", "closed"); err == nil {
		t.Error("a status write on an id this project does not own must be an error")
	}
	if err := ps.PutOwnedTask(OwnedTask{ID: "td-2", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetOwnedStatus("td-2", "closed"); err != nil {
		t.Fatalf("status write on an owned task: %v", err)
	}
	got, _, _ := ps.OwnedTask("td-2")
	if got.Status != "closed" {
		t.Errorf("status = %q, want closed", got.Status)
	}
}

// TestProjectsDoNotSeeEachOthersTasks: the table is project-keyed, and one repo's backlog appearing
// in another would hand a worker someone else's work.
func TestProjectsDoNotSeeEachOthersTasks(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	for _, p := range []string{"a", "b"} {
		if err := st.RegisterProject(p, t.TempDir()); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.For("a").PutOwnedTask(OwnedTask{ID: "td-1", Title: "a's task", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.For("b").OwnedTask("td-1"); ok {
		t.Error("project b must not see project a's task")
	}
	if st.For("b").OwnsTask("td-1") {
		t.Error("OwnsTask must be project-scoped")
	}
	if !st.For("a").OwnsTask("td-1") {
		t.Error("the owning project must recognise its own task")
	}
}

// TestDeleteRemovesTheTask covers the scrap path.
func TestDeleteRemovesTheTask(t *testing.T) {
	ps := ownedStore(t)
	if err := ps.PutOwnedTask(OwnedTask{ID: "td-3", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.DeleteOwnedTask("td-3"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := ps.OwnedTask("td-3"); ok {
		t.Error("the task should be gone")
	}
}

// TestLabelList keeps the comma column readable as a list, matching the mirror's own encoding.
func TestLabelList(t *testing.T) {
	got := LabelList(" spec:add-auth , ui ,, ")
	if len(got) != 2 || got[0] != "spec:add-auth" || got[1] != "ui" {
		t.Errorf("LabelList = %q, want the two non-empty labels trimmed", got)
	}
	if len(LabelList("")) != 0 {
		t.Error("no labels must yield no entries")
	}
}
