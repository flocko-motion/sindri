package fleet

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	flowpr "github.com/flo-at/sindri/internal/hub/flow/pr"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/world/store"

	_ "modernc.org/sqlite"
)

// approvedPR seeds one approved merge intent in a project with nothing else going on, handing back
// the database path so a test can age a state stamp (-> agePRState).
func approvedPR(t *testing.T) (*Engine, *store.ProjectStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	if err := ps.UpsertTask(store.Task{ID: "td-1", Title: "a task", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "td-1", Agent: "bombur", Branch: "sd-1", Base: "main", Status: "approved"}); err != nil {
		t.Fatal(err)
	}
	return newEngine(t, st, &flowtest.Hub{Root: root}), ps, path
}

// agePRState puts a merge intent's state stamp before this process started, which is exactly what
// a hub that died mid-merge leaves behind: the state written, nothing running, and an entry time
// older than the machine now looking at it.
func agePRState(t *testing.T, path, id string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE prs SET state_since=? WHERE id=?`,
		time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), id); err != nil {
		t.Fatal(err)
	}
}

// TestAMergeNobodyAskedForNeverRuns is the hard gate from the other side: the intent is the ONE way
// into merging, so a pull request that nobody typed `merge` for sits approved however often it is
// looked at.
func TestAMergeNobodyAskedForNeverRuns(t *testing.T) {
	e, ps, _ := approvedPR(t)

	e.LookPR("repo", "pr-1")

	if state, _, _ := ps.PRState("pr-1"); state == flowpr.Merging || state == flowpr.Merged {
		t.Errorf("pr-1 reached %q with nobody having asked — only a human's merge writes that intent", state)
	}
}

// TestAHubThatDiedMidMergeRecoversThroughStuck: a merge is not atomic, so a hub that stopped half
// way through leaves nobody knowing whether the base carries it. The merge intent is recovered by
// the merging state's own orphan exit — a state entered before this process started, with nothing
// running — rather than by a sweep at startup that only ever ran once.
func TestAHubThatDiedMidMergeRecoversThroughStuck(t *testing.T) {
	e, ps, path := approvedPR(t)
	// What a hub that died mid-merge leaves behind: the state written, its stamp older than this
	// process, and no action in flight.
	if err := ps.SetPRState("pr-1", flowpr.Merging); err != nil {
		t.Fatal(err)
	}
	agePRState(t, path, "pr-1")

	e.LookPR("repo", "pr-1")

	state, _, err := ps.PRState("pr-1")
	if err != nil {
		t.Fatal(err)
	}
	if state != flowpr.Stuck {
		t.Errorf("pr-1 stands in %q, want %q — a half-merge asks for a human rather than reading in-flight for ever",
			state, flowpr.Stuck)
	}
	pr, _, _ := ps.GetPR("pr-1")
	if pr.Status != "merge-failed" {
		t.Errorf("status = %q, want merge-failed — the board must show what the state says", pr.Status)
	}
}
