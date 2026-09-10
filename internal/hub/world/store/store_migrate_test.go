package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestAColumnAddedToAnExistingTableIsMigrated: every test until now opened a FRESH database, where
// CREATE TABLE writes the current schema and no migration is needed — so a column added to the
// schema without its ALTER passed every test and failed on the only database that matters, the
// user's. `closed` did exactly that: "table task_priority has no column named closed", after the
// checkpoint fix it exists to serve.
func TestAColumnAddedToAnExistingTableIsMigrated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.db")
	// The table as an older build left it: no tier, no closed.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE task_priority (
		project TEXT NOT NULL, id TEXT NOT NULL, priority TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (project, id))`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	st, err := Open(path)
	if err != nil {
		t.Fatalf("opening a database from an older build: %v", err)
	}
	defer st.Close()
	if err := st.RegisterProject("proj", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	// Each override the current build writes must reach a column that is actually there.
	if err := ps.SetClosedOverride("os-1"); err != nil {
		t.Errorf("SetClosedOverride against a migrated table: %v", err)
	}
	if err := ps.SetTierOverride("os-1", "senior"); err != nil {
		t.Errorf("SetTierOverride against a migrated table: %v", err)
	}
	if ov, err := ps.ClosedOverrides(); err != nil || !ov["os-1"] {
		t.Errorf("the ending did not survive: %v (err %v)", ov, err)
	}
}
