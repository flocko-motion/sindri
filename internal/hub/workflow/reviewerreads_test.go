package workflow

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// reviewerEngine seeds a small backlog with a hierarchy and a spec label, plus the PR a reviewer
// was handed, and returns the caller a reviewer arrives as.
func reviewerEngine(t *testing.T) (*Engine, registry.Caller, *store.ProjectStore) {
	t.Helper()
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
	// Owned rows, not cache rows: an sd- id is read from the owned table (-> TaskInfo), and the sync
	// CmdTasks runs first rebuilds the cache from those, so a cache-only seed would be swept away.
	for _, task := range []store.OwnedTask{
		{ID: "sd-parent", Title: "the feature", Status: "open", Priority: "P1"},
		{ID: "sd-1", Title: "the reviewed work", Status: "open", Priority: "P1",
			Description: "what it was for", Labels: "spec:view-tui"},
		{ID: "sd-2", Title: "the sibling", Status: "open", Priority: "P2"},
		{ID: "sd-child", Title: "a child of the work", Status: "open", Priority: "P2"},
		{ID: "sd-elsewhere", Title: "unrelated backlog", Status: "open", Priority: "P3"},
	} {
		if err := ps.PutOwnedTask(task); err != nil {
			t.Fatal(err)
		}
		// The cache too, which is what a plain read sees. In a live hub the sync rebuilds it from
		// the owned rows; nothing has synced here, and `show` reads the cache rather than writing.
		if err := ps.UpsertTask(store.Task{ID: task.ID, Title: task.Title, Status: task.Status,
			Priority: task.Priority, Description: task.Description, Labels: task.Labels}); err != nil {
			t.Fatal(err)
		}
	}
	// Parentage lives in its own table, for every task rather than only owned ones.
	for child, parent := range map[string]string{"sd-1": "sd-parent", "sd-2": "sd-parent", "sd-child": "sd-1"} {
		if err := ps.SetParent(child, parent); err != nil {
			t.Fatal(err)
		}
	}
	if err := ps.PutPR(store.PR{ID: "pr-sd-1", Task: "sd-1", Agent: "bombur", Branch: "sd-1", Base: "main", Status: "submitted"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutAgent(store.Agent{Name: "rune", Role: "reviewer", Workspace: ".worktrees/rune"}); err != nil {
		t.Fatal(err)
	}
	return New(st, &stubDeps{root: root}), registry.Caller{Project: "proj", Agent: "rune", Role: "reviewer"}, ps
}

// reviewerEngineWithComments is reviewerEngine with a thread on the reviewed task; comments live
// outside the task row, so they are served by deps rather than upserted with it.
func reviewerEngineWithComments(t *testing.T, thread []store.Comment) (*Engine, registry.Caller, *store.ProjectStore) {
	t.Helper()
	e, c, ps := reviewerEngine(t)
	e.deps.(*stubDeps).comments = map[string][]store.Comment{"sd-1": thread}
	return e, c, ps
}

// TestReviewerReadsTheTaskItIsReviewing is the gap. A reviewer judged a diff against the
// architecture doc and its own taste, because the one thing saying what the work was FOR — the task
// — was unreadable to it. It could answer "is this good code" and never "does this do what was
// asked", leaving intent unchecked by anyone but the human at merge.
func TestReviewerReadsTheTaskItIsReviewing(t *testing.T) {
	e, c, _ := reviewerEngine(t)
	var out bytes.Buffer
	if _, err := e.CmdTasks(c, []string{"sd-1"}, &out); err != nil {
		t.Fatalf("CmdTasks: %v", err)
	}
	got := out.String()
	for _, want := range []string{"sd-1", "the reviewed work", "what it was for"} {
		if !strings.Contains(got, want) {
			t.Errorf("the reviewer cannot read %q:\n%s", want, got)
		}
	}
}

// TestReviewerSeesTheHierarchy: the parent and children are what let a reviewer tell a genuine gap
// from a boundary another task deliberately owns, and whether a subtask is one of several sharing a
// branch. Reading the task alone would still leave that invisible.
func TestReviewerSeesTheHierarchy(t *testing.T) {
	e, c, _ := reviewerEngine(t)
	var out bytes.Buffer
	if _, err := e.CmdTasks(c, []string{"sd-1"}, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "sd-parent") {
		t.Errorf("the parent is not shown:\n%s", got)
	}
	if !strings.Contains(got, "sd-child") {
		t.Errorf("the children are not shown:\n%s", got)
	}
	// And the siblings, via the tree listing.
	out.Reset()
	if _, err := e.CmdTasks(c, []string{"list"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sd-parent", "sd-2", "sd-elsewhere"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the reviewer's backlog listing omits %q:\n%s", want, out.String())
		}
	}
}

// TestReviewerReadsTheComments: sd-c7ed74 in this very backlog was redirected entirely by a comment
// while its body still described the superseded approach. A reviewer reading only the body would
// have measured that work against a plan already abandoned.
func TestReviewerReadsTheComments(t *testing.T) {
	e, c, _ := reviewerEngineWithComments(t, []store.Comment{
		{Source: "sindri", Author: "the user", Body: "actually, do it the other way", CreatedAt: "2026-08-12T09:00:00Z"},
	})
	var out bytes.Buffer
	if _, err := e.CmdTasks(c, []string{"sd-1"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "do it the other way") {
		t.Errorf("the correction is invisible to the reviewer:\n%s", out.String())
	}
}

// TestReviewerFindsTheSpecLabel: 05-workflow requires a reviewer to verify a `spec:<name>` task
// against every requirement and scenario in that spec. The label lives on the task, so the
// requirement was not implementable with the surface the role had.
func TestReviewerFindsTheSpecLabel(t *testing.T) {
	e, c, _ := reviewerEngine(t)
	var out bytes.Buffer
	if _, err := e.CmdTasks(c, []string{"sd-1"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "spec:view-tui") {
		t.Errorf("the reviewer cannot discover which spec applies:\n%s", out.String())
	}
}

// TestShowPRNamesItsTask: the host's PR detail renders the linked task, and the agent-facing `show`
// did not — so the surface a reviewer is pointed at showed the diff and never what it was for.
func TestShowPRNamesItsTask(t *testing.T) {
	e, c, _ := reviewerEngine(t)
	var out bytes.Buffer
	// The diff itself needs a real repo; the metadata is printed before that fails, which is the
	// part under test.
	_, _ = e.CmdShowPR(c, []string{"pr-sd-1"}, &out)
	got := out.String()
	if !strings.Contains(got, "sd-1") || !strings.Contains(got, "the reviewed work") {
		t.Errorf("`show` does not name the linked task:\n%s", got)
	}
}

// TestTheReviewerIsToldItCanRead: access nobody mentions is access nobody uses. Both routes into a
// review — the directive and the prompt — have to point at the verbs, or the reviewer keeps judging
// the diff against the architecture doc alone.
func TestTheReviewerIsToldItCanRead(t *testing.T) {
	dir := DirReview("pr-sd-1", "sd-1", "the reviewed work", "dwalin", "ARCHITECTURE.md")
	for _, want := range []string{"sd-1", "the reviewed work", "sindri task sd-1", "task list", "COMMENTS", "spec:"} {
		if !strings.Contains(dir, want) {
			t.Errorf("the directive does not mention %q:\n%s", want, dir)
		}
	}
	for _, want := range []string{"sindri task", "task list", "comment", "spec:"} {
		if !strings.Contains(DefaultReviewPrompt, want) {
			t.Errorf("the default review prompt does not mention %q:\n%s", want, DefaultReviewPrompt)
		}
	}
}

// TestDirReviewSurvivesAnUnreadableTitle: a title that will not load must not stop a review being
// handed out — the reviewer still needs the PR id and the verbs.
func TestDirReviewSurvivesAnUnreadableTitle(t *testing.T) {
	dir := DirReview("pr-sd-1", "sd-1", "", "dwalin", "ARCHITECTURE.md")
	if !strings.Contains(dir, "pr-sd-1") || !strings.Contains(dir, "sindri task sd-1") {
		t.Errorf("a missing title cost the directive its substance:\n%s", dir)
	}
}

// promptEngine builds an engine whose state dir is a temp one, so a test can place (or not place) a
// review-prompt.txt without touching the real install.
func promptEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	t.Setenv("SINDRI_HOME", t.TempDir())
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	return New(st, &stubDeps{root: root}), reviewPromptPath("proj")
}

// writePrompt places a review-prompt.txt, as an older sindri did on first use.
func writePrompt(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestTheDefaultPromptIsNotWrittenOut is the cause. Seeding the file on first use meant the default
// could never be improved again: every project that had ever requested a review held a copy, so a
// better one shipped to new installs only — and silently, since nothing reports a stale seed.
func TestTheDefaultPromptIsNotWrittenOut(t *testing.T) {
	e, path := promptEngine(t)
	got, err := e.ReviewPrompt("proj")
	if err != nil {
		t.Fatal(err)
	}
	if got != DefaultReviewPrompt {
		t.Errorf("an unconfigured project should get the built-in default, got %q", got)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("the default was written to disk, which is what freezes it forever")
	}
}

// TestAnImprovedDefaultReachesAnExistingProject is the half that stopping the write does not fix on
// its own: an established install already HAS the file, so treating it as an override would keep
// serving the stale seed. A file byte-matching something sindri wrote was never a decision.
func TestAnImprovedDefaultReachesAnExistingProject(t *testing.T) {
	e, path := promptEngine(t)
	writePrompt(t, path, "Review this PR for correctness, clarity, and fit to the task. Flag bugs, missing tests, and anything that should change.")

	got, err := e.ReviewPrompt("proj")
	if err != nil {
		t.Fatal(err)
	}
	if got != DefaultReviewPrompt {
		t.Errorf("a project holding the old seed still gets it:\n%s", got)
	}
	// And the improvement is the point: it must now say how to read the task.
	if !strings.Contains(got, "sindri task") {
		t.Errorf("the prompt reaching an existing project does not point at the task:\n%s", got)
	}
}

// TestAnEditedPromptStillWins: the file remains the way to override the instruction, and someone
// who wrote their own must not have it replaced by a default that thinks it knows better.
func TestAnEditedPromptStillWins(t *testing.T) {
	e, path := promptEngine(t)
	writePrompt(t, path, "Only check the tests.")
	got, err := e.ReviewPrompt("proj")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Only check the tests." {
		t.Errorf("an edited prompt should win, got %q", got)
	}
}

// TestTheCurrentDefaultOnDiskIsAlsoJustASeed: a project seeded with today's default must not be
// frozen at it either, or this recurs the next time the default improves.
func TestTheCurrentDefaultOnDiskIsAlsoJustASeed(t *testing.T) {
	e, path := promptEngine(t)
	writePrompt(t, path, DefaultReviewPrompt)
	if !isSeededPrompt(strings.TrimSpace(DefaultReviewPrompt)) {
		t.Error("the current default must count as a seed, or the next improvement is stuck again")
	}
	got, err := e.ReviewPrompt("proj")
	if err != nil {
		t.Fatal(err)
	}
	if got != DefaultReviewPrompt {
		t.Errorf("got %q", got)
	}
}

// TestShowPRDoesNotWrite: displaying a PR must not mutate anything. Resolving the task through
// TaskInfo would reconcile and re-cache it, putting a write behind a read verb — cheap to avoid,
// since the cache already holds the two fields shown.
func TestShowPRDoesNotWrite(t *testing.T) {
	e, c, ps := reviewerEngine(t)
	// Drop the cache row: only the owned one remains, so any write would put it back.
	if err := ps.RemoveTask("sd-1"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	_, _ = e.CmdShowPR(c, []string{"pr-sd-1"}, &out)
	if _, ok, _ := ps.GetTask("sd-1"); ok {
		t.Error("showing a PR wrote the task back into the cache — a display verb must not mutate")
	}
	// It still names the task from what it has.
	if !strings.Contains(out.String(), "sd-1") {
		t.Errorf("`show` should still name the task id:\n%s", out.String())
	}
}

// TestTheReviewDirectiveNamesTheAuthor is the gap: a reviewer was handed a PR and never told whose
// work it was. The name was reachable — `sindri show <pr>` prints it — but only by going looking,
// and nothing suggested looking, so verdicts were written about nobody.
func TestTheReviewDirectiveNamesTheAuthor(t *testing.T) {
	dir := DirReview("pr-sd-1", "sd-1", "the reviewed work", "dwalin", "ARCHITECTURE.md")
	if !strings.Contains(dir, "dwalin") {
		t.Errorf("the directive must name the author:\n%s", dir)
	}
	// Named where the reviewer is told WHAT to do, not in a footnote: the first line is the one
	// sentence every reviewer reads.
	first := strings.SplitN(dir, "\n", 2)[0]
	if !strings.Contains(first, "dwalin") {
		t.Errorf("the author belongs in the opening line, got %q", first)
	}
	if !strings.Contains(first, "sd-1") {
		t.Errorf("the task is still named beside the author, got %q", first)
	}
}

// TestAnAuthorlessPRStillReadsAsASentence: an older PR record carries no author, and "'s work on"
// with nothing in front of it would read worse than the plain task line it replaced.
func TestAnAuthorlessPRStillReadsAsASentence(t *testing.T) {
	dir := DirReview("pr-sd-1", "sd-1", "the reviewed work", "", "ARCHITECTURE.md")
	first := strings.SplitN(dir, "\n", 2)[0]
	if strings.Contains(first, "'s work") {
		t.Errorf("with no author the line must not claim one, got %q", first)
	}
	if !strings.Contains(first, "task sd-1") {
		t.Errorf("it falls back to naming the task, got %q", first)
	}
}
