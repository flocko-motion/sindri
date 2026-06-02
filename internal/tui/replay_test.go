package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain pins the test process timezone to UTC so the View's
// timestamp formatting is identical across machines, keeping the
// golden frames stable. It also gives every test the same starting
// world.
func TestMain(m *testing.M) {
	os.Setenv("TZ", "UTC")
	time.Local = time.UTC
	os.Exit(m.Run())
}

// AssertGolden compares <captureDir>/<name>.txt against the committed
// golden at testdata/frames/<name>.txt. When GO_UPDATE_GOLDENS=1 (or the
// golden does not yet exist), it (re)writes the golden from the capture
// so intentional changes can be reviewed as a single diff.
func AssertGolden(t *testing.T, captureDir, name string) {
	t.Helper()
	capPath := filepath.Join(captureDir, name+".txt")
	got, err := os.ReadFile(capPath)
	if err != nil {
		t.Fatalf("missing capture %s: %v", name, err)
	}
	goldenDir := filepath.Join("testdata", "frames")
	goldPath := filepath.Join(goldenDir, name+".txt")
	want, err := os.ReadFile(goldPath)
	switch {
	case os.IsNotExist(err) || os.Getenv("GO_UPDATE_GOLDENS") == "1":
		if mkErr := os.MkdirAll(goldenDir, 0o755); mkErr != nil {
			t.Fatalf("mkdir goldens: %v", mkErr)
		}
		if wErr := os.WriteFile(goldPath, got, 0o644); wErr != nil {
			t.Fatalf("write golden %s: %v", goldPath, wErr)
		}
		if err == nil {
			t.Logf("updated golden %s", goldPath)
		}
		return
	case err != nil:
		t.Fatalf("read golden %s: %v", goldPath, err)
	}
	if string(got) != string(want) {
		t.Errorf("golden drift in %s — re-run with GO_UPDATE_GOLDENS=1 to refresh\n"+
			"--- want (%d bytes)\n%s\n--- got (%d bytes)\n%s",
			name, len(want), want, len(got), got)
	}
}

// TestReplay_BasicListAndFilter sanity-checks the engine end-to-end with
// substring assertions (cheap, independent of golden churn): build the
// fixture, capture default list, cycle filter to "all" (closed appears),
// then "closed only", then back.
func TestReplay_BasicListAndFilter(t *testing.T) {
	dir := t.TempDir()
	script := "(capture list-default) f (capture list-all) f (capture list-closed) f (capture list-back)"
	if err := Replay(script, SimpleFixture(), dir); err != nil {
		t.Fatalf("replay: %v", err)
	}

	readTxt := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name+".txt"))
		if err != nil {
			t.Fatalf("missing capture %s: %v", name, err)
		}
		return string(b)
	}
	containsAll := func(s string, subs ...string) bool {
		for _, sub := range subs {
			if !strings.Contains(s, sub) {
				return false
			}
		}
		return true
	}

	def := readTxt("list-default")
	if !containsAll(def, "td-aaaaaa", "td-bbbbbb") {
		t.Errorf("default list missing open/in-progress items:\n%s", def)
	}
	if strings.Contains(def, "td-cccccc") {
		t.Errorf("default list should hide closed td-cccccc but didn't:\n%s", def)
	}

	all := readTxt("list-all")
	if !containsAll(all, "td-aaaaaa", "td-bbbbbb", "td-cccccc") {
		t.Errorf("FilterAll missing some items:\n%s", all)
	}

	closed := readTxt("list-closed")
	if !strings.Contains(closed, "td-cccccc") {
		t.Errorf("FilterClosed missing closed task:\n%s", closed)
	}
	if strings.Contains(closed, "td-aaaaaa") {
		t.Errorf("FilterClosed should hide open td-aaaaaa but didn't:\n%s", closed)
	}

	back := readTxt("list-back")
	if strings.Contains(back, "td-cccccc") {
		t.Errorf("after cycling back, closed should be hidden again:\n%s", back)
	}
}

// TestReplayGoldens_Mock captures the work list against MockFixture, which has
// mixed task types (bug / feature / chore / epic / task) and an epic with two
// children — exercising the type-indicator and hierarchy requirements added
// by enrich-work-list-display.
func TestReplayGoldens_Mock(t *testing.T) {
	dir := t.TempDir()
	if err := Replay("(capture list-mock)", MockFixture(), dir); err != nil {
		t.Fatalf("replay: %v", err)
	}
	AssertGolden(t, dir, "list-mock")
}

// TestReplayGoldens_LoadingState captures the startup window before any
// refresh has applied. The fixture has empty Issues and Workers and the
// engine respects LoadingState=true by leaving m.loaded at false, so both
// panels render their "Loading…" placeholders instead of the empty-state
// "No tasks" / "No workers" text.
func TestReplayGoldens_LoadingState(t *testing.T) {
	dir := t.TempDir()
	fx := Fixture{Width: 100, Height: 30, LoadingState: true}
	script := "(capture list-loading) W (capture workers-loading)"
	if err := Replay(script, fx, dir); err != nil {
		t.Fatalf("replay: %v", err)
	}
	AssertGolden(t, dir, "list-loading")
	AssertGolden(t, dir, "workers-loading")
}

// TestReplay_UnknownToken proves bad scripts fail with the offending name.
func TestReplay_UnknownToken(t *testing.T) {
	err := Replay("(frobnicate)", SimpleFixture(), t.TempDir())
	if err == nil {
		t.Fatalf("expected an error for unknown directive, got nil")
	}
	if !strings.Contains(err.Error(), "frobnicate") {
		t.Errorf("error should name the offending token: %v", err)
	}
}

// TestReplayGoldens drives the TUI through every recently-touched state
// and compares each capture against a committed golden under
// testdata/frames/. To regenerate after an intentional layout change,
// run: GO_UPDATE_GOLDENS=1 go test ./internal/tui/ -run TestReplayGoldens
//
// Backlog rows under the default filter (FilterOpen), in order:
//
//	row 0: os-... spec-only (auth-refactor)
//	row 1: td-aaaaaa open
//	row 2: td-aaaaaa unmet gate (navigable, not a PR)
//	row 3: td-bbbbbb in_progress (worker brokkr)
//	row 4: pr-td-bbbbbb (isPR — moveCursor skips this)
//	row 5: td-bbbbbb met gate
//
// So three `down`s land on the in-progress task (rows 0→1→2→3); the
// isPR row is skipped automatically by moveCursor.
func TestReplayGoldens(t *testing.T) {
	dir := t.TempDir()
	script := strings.Join([]string{
		"(capture list-default)",                  // 1. default filter
		"f (capture list-all)",                    // 2. all
		"f (capture list-closed)",                 // 3. closed only
		"f",                                       // back to open
		"down down down enter (capture detail-task)", // 4. in-progress task detail
		"esc",
		"up up up enter (capture detail-spec)",    // 5. spec-only detail
		"esc",
		"W (capture workers)",                     // 6. workers view (role column)
		"T",                                       // back to backlog (cursor at row 0)
		"down down down enter",                    // re-open in-progress detail
		"m (capture merge-confirm)",               // 7. merge confirmation modal
		"esc",                                     // dismiss confirm (stays in detail)
		"x (capture reject-reason)",               // 8. reject-reason input bar
		"esc",
		"s (capture status-pick)",                 // 9. status picker (cursor on current = in_progress)
		"right (capture status-pick-moved)",       // 10. picker after one right-arrow (cursor on in_review)
		"esc",                                     // close picker, still in detail
		"esc",                                     // close detail, back to list (cursor at row 3 = in_progress)
		"s (capture status-pick-from-list)",       // 11. picker opened from the LIST view
		"esc",
		"m (capture list-move-active)",            // 12. move mode — current row painted red as "in movement"
		"esc",                                     // cancel move
		"up up a (capture list-approve-no-pr)",    // 13. cursor moves up to td-aaaaaa (open, no PR) — pressing 'a' surfaces the visible "no PR yet" notification
		"up n (capture create-spec-linked)",       // 14. cursor moves up to the spec-only row (auth-refactor); pressing 'n' opens the create-task modal pre-linked to that spec
		"esc",                                     // dismiss create modal, back on spec row
		"x (capture abandon-spec-confirm)",        // 15. x on a spec-only row → abandon confirm (impact line in the bottom bar)
	}, " ")
	if err := Replay(script, SimpleFixture(), dir); err != nil {
		t.Fatalf("replay: %v", err)
	}
	for _, name := range []string{
		"list-default", "list-all", "list-closed",
		"detail-task", "detail-spec",
		"workers",
		"merge-confirm", "reject-reason",
		"status-pick", "status-pick-moved", "status-pick-from-list",
		"list-move-active",
		"list-approve-no-pr",
		"create-spec-linked",
		"abandon-spec-confirm",
	} {
		AssertGolden(t, dir, name)
	}
}
