package workflow

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// plannerEngine builds an engine over a throwaway store with one cached task in the given
// approval state, and returns the caller a planner would arrive as.
func plannerEngine(t *testing.T, id, approval string) (*Engine, registry.Caller, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("proj", t.TempDir()); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("proj")
	if err := ps.UpsertTask(store.Task{ID: id, Title: "flat proposal", Status: "open"}); err != nil {
		t.Fatalf("upsert task: %v", err)
	}
	if approval != "" {
		if err := ps.SetApproval(id, approval, ""); err != nil {
			t.Fatalf("set approval: %v", err)
		}
	}
	e := New(st, &stubDeps{root: t.TempDir()})
	return e, registry.Caller{Project: "proj", Agent: "galar", Role: "planner"}, ps
}

// plannerOwnedTask builds an engine holding one task sindri owns, in the given approval state, and
// returns its minted id alongside the planner caller. CreateTask rather than a cached row: an edit
// to a title or body writes owned_tasks, and a task that only exists in the read model absorbs none
// of it.
func plannerOwnedTask(t *testing.T, approval string) (*Engine, registry.Caller, *store.ProjectStore, string, *stubDeps) {
	t.Helper()
	e, c, ps := plannerEngine(t, "td-parent", "")
	id, err := e.CreateTask("proj", TaskSpec{Title: "flat proposal", Description: "as first written", Type: "task"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if approval != "" {
		if err := ps.SetApproval(id, approval, ""); err != nil {
			t.Fatalf("set approval: %v", err)
		}
	}
	return e, c, ps, id, e.deps.(*stubDeps)
}

// TestAnyTaskIsEditableAndGoesBackForAVerdict is the rule, and it is one rule: a planner edits
// whatever it can see, and every edit returns the task to awaiting-review. Approval records that
// the user has READ this task — an edit makes that record untrue, so it is cleared. It is not
// permission the planner has to hold, which is what the old pending-only refusal took it for.
func TestAnyTaskIsEditableAndGoesBackForAVerdict(t *testing.T) {
	for _, approval := range []string{"approved", "rejected", "pending", ""} {
		e, c, ps, id, _ := plannerOwnedTask(t, approval)
		var out bytes.Buffer
		code, err := e.CmdEditTask(c, []string{id, "a sharper title"}, &out)
		if err != nil {
			t.Fatalf("approval %q: unexpected error: %v", approval, err)
		}
		if code != 0 {
			t.Errorf("approval %q: the edit should be allowed, got exit %d: %s", approval, code, out.String())
		}
		if got, _ := ps.GetApproval(id); got != "pending" {
			t.Errorf("approval %q: after an edit the task should await a fresh verdict, got %q", approval, got)
		}
		if tk, _, _ := ps.GetTask(id); tk.Title != "a sharper title" {
			t.Errorf("approval %q: the edit did not land, title is %q", approval, tk.Title)
		}
	}
}

// TestEditPausesRelease: the same act stops the task being handed out, which is the point rather
// than a side effect — a task whose definition just changed must not be grabbed before the user
// has seen the change.
func TestEditPausesRelease(t *testing.T) {
	e, c, ps, id, _ := plannerOwnedTask(t, "approved")
	if err := ps.SetOwnedPriority(id, "P1"); err != nil {
		t.Fatalf("rate: %v", err)
	}
	e.refreshCachedTask("proj", id)
	if leaves, err := ps.OpenLeaves(); err != nil || !hasTask(leaves, id) {
		t.Fatalf("an approved, rated task should be claimable to begin with (err=%v): %v", err, leaves)
	}
	var out bytes.Buffer
	if code, err := e.CmdEditTask(c, []string{id, "--body", "the premise was wrong"}, &out); code != 0 || err != nil {
		t.Fatalf("edit: code=%d err=%v out=%s", code, err, out.String())
	}
	leaves, err := ps.OpenLeaves()
	if err != nil {
		t.Fatal(err)
	}
	if hasTask(leaves, id) {
		t.Error("an edited task must stop being handed out until the user has seen the change")
	}
}

// hasTask reports whether id is among these tasks.
func hasTask(tasks []store.Task, id string) bool {
	for _, tk := range tasks {
		if tk.ID == id {
			return true
		}
	}
	return false
}

// TestEditIsRecordedOnTheTask: the user has to be able to see WHAT changed, not merely that
// something did — and the old value survives nowhere else once the write lands. It goes on the
// task, where the user reads it, rather than into the planner's own activity log.
func TestEditIsRecordedOnTheTask(t *testing.T) {
	e, c, _, id, deps := plannerOwnedTask(t, "approved")
	var out bytes.Buffer
	if code, err := e.CmdEditTask(c, []string{id, "--body", "the premise was wrong"}, &out); code != 0 || err != nil {
		t.Fatalf("edit: code=%d err=%v out=%s", code, err, out.String())
	}
	if len(deps.posted) != 1 {
		t.Fatalf("the edit should be recorded on the task, got %d comments", len(deps.posted))
	}
	got := deps.posted[0]
	if got.SourceRef != id || got.Author != "galar" {
		t.Errorf("recorded on %q by %q, want %s by galar", got.SourceRef, got.Author, id)
	}
	for _, want := range []string{"description", "as first written", "the premise was wrong"} {
		if !strings.Contains(got.Body, want) {
			t.Errorf("the record should carry %q:\n%s", want, got.Body)
		}
	}
}

// TestEditKeepsTheVerdictItReplaces: a rejection's reason lives in the approval row and nowhere
// else, and clearing that row is exactly what an edit does. Revising a rejected task is the
// ordinary flow — the reason has to survive into the record, or the next reader sees a task that
// bounced and no word of why.
func TestEditKeepsTheVerdictItReplaces(t *testing.T) {
	e, c, ps, id, deps := plannerOwnedTask(t, "")
	if err := ps.SetApproval(id, "rejected", "the premise is false"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code, err := e.CmdEditTask(c, []string{id, "--body", "rewritten on a true premise"}, &out); code != 0 || err != nil {
		t.Fatalf("edit: code=%d err=%v out=%s", code, err, out.String())
	}
	if len(deps.posted) != 1 || !strings.Contains(deps.posted[0].Body, "the premise is false") {
		t.Errorf("the record should carry the verdict it replaced: %v", deps.posted)
	}
}

// TestEditTellsTheHolder: a worker holds the task as it read it at claim time, so an edit that
// nobody tells it about leaves it building to a brief that no longer exists. It is told for the
// held task and for the feature it holds alike — both are its unit of work.
func TestEditTellsTheHolder(t *testing.T) {
	for _, held := range []string{"task", "container"} {
		e, c, ps, id, deps := plannerOwnedTask(t, "approved")
		deps.alive = true
		if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker"}); err != nil {
			t.Fatal(err)
		}
		st := store.AgentState{Agent: "eitri", Branch: id, Phase: "working"}
		if held == "task" {
			st.Task = id
		} else {
			st.Container = id
		}
		if err := ps.SetState(st); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if code, err := e.CmdEditTask(c, []string{id, "--body", "the premise was wrong"}, &out); code != 0 || err != nil {
			t.Fatalf("%s: edit: code=%d err=%v out=%s", held, code, err, out.String())
		}
		if len(deps.injected) != 1 || deps.injected[0] != "eitri" {
			t.Fatalf("%s: the holder should be told, injected: %v", held, deps.injected)
		}
		// It has to be able to act on it: which task, that it changed, and that the work is still its
		// own to finish — a task showing "pending" again otherwise reads as one taken away.
		for _, want := range []string{id, "description", "sindri task " + id, "you finish and submit it"} {
			if !strings.Contains(deps.injectedText[0], want) {
				t.Errorf("%s: the note should carry %q:\n%s", held, want, deps.injectedText[0])
			}
		}
		if !strings.Contains(out.String(), "eitri") {
			t.Errorf("%s: the planner should be told who holds it:\n%s", held, out.String())
		}
	}
}

// TestAFailedRecordStillTellsTheHolder: the record failing is precisely when the holder cannot find
// out any other way, so it must not be the path where nobody tells it. The planner is told the task
// carries no record, since it is then the only one who can put that right.
func TestAFailedRecordStillTellsTheHolder(t *testing.T) {
	e, c, ps, id, deps := plannerOwnedTask(t, "approved")
	deps.alive, deps.postFails = true, true
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: id, Branch: id, Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code, err := e.CmdEditTask(c, []string{id, "--body", "corrected"}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code == 0 {
		t.Errorf("a failed record should be reported as a failure: %s", out.String())
	}
	if len(deps.injected) != 1 || deps.injected[0] != "eitri" {
		t.Fatalf("the holder should still be told, injected: %v", deps.injected)
	}
	for _, want := range []string{"no record", "meeting room", "eitri"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the reply should carry %q:\n%s", want, out.String())
		}
	}
	// The edit itself stands, verdict and all — reporting otherwise would send the planner to undo
	// a write that landed.
	if got, _ := ps.GetApproval(id); got != "pending" {
		t.Errorf("the edit and its un-approval should stand, approval is %q", got)
	}
}

// TestAnEditedTaskIsStillItsHolders: un-approving withdraws a task from the pools work is handed
// out FROM; it says nothing about finishing work already in hand. A worker stranded because a
// planner corrected a line in its brief would be the whole change made worthless.
func TestAnEditedTaskIsStillItsHolders(t *testing.T) {
	e, c, ps, id, deps := plannerOwnedTask(t, "approved")
	deps.alive = true
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: id, Branch: id, Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code, err := e.CmdEditTask(c, []string{id, "--body", "corrected"}, &out); code != 0 || err != nil {
		t.Fatalf("edit: code=%d err=%v out=%s", code, err, out.String())
	}
	d, err := e.AgentDirective(t.Context(), "proj", "eitri")
	if err != nil {
		t.Fatalf("directive: %v", err)
	}
	if !strings.Contains(d, id) || !strings.Contains(d, "submit") {
		t.Errorf("the holder should still be told to finish and submit %s, got: %s", id, d)
	}
}

// TestEditTaskRefusesAnUnknownId: the approval read that used to stand here refused an absent task
// by accident. Without a check of its own, an edit that writes nothing reports success.
func TestEditTaskRefusesAnUnknownId(t *testing.T) {
	e, c, _ := plannerEngine(t, "td-1", "pending")
	var out bytes.Buffer
	code, err := e.CmdEditTask(c, []string{"td-nope", "a new title"}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code == 0 || !strings.Contains(out.String(), "no such task") {
		t.Errorf("editing a task that isn't there should be refused, got exit %d: %s", code, out.String())
	}
}

// TestEditTaskLeavesRatingToPrioritiseTask: one field must not have two verbs with opposite
// consequences. An edit returns the task for a verdict; re-ordering leaves the verdict standing —
// so a rating carried in here would mean different things depending on which verb was reached for.
func TestEditTaskLeavesRatingToPrioritiseTask(t *testing.T) {
	e, c, ps, id, _ := plannerOwnedTask(t, "approved")
	var out bytes.Buffer
	code, err := e.CmdEditTask(c, []string{id, "--priority", "high", "a sharper title"}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code == 0 || !strings.Contains(out.String(), "prioritise-task") {
		t.Errorf("a rating should be sent to prioritise-task, got exit %d: %s", code, out.String())
	}
	// And nothing at all was written — not the title that came with it, not the verdict.
	if tk, _, _ := ps.GetTask(id); tk.Title != "flat proposal" {
		t.Errorf("the refused call still edited the task: %q", tk.Title)
	}
	if got, _ := ps.GetApproval(id); got != "approved" {
		t.Errorf("the refused call still cleared the verdict: %q", got)
	}
}

// TestEditingWhatSindriDoesNotOwnSaysSo: a mirrored task's content belongs to its own source, so an
// edit to it writes nothing. Reported from what the store says actually moved — echoing the request
// back would claim a change that never happened, and would un-approve the task for it.
func TestEditingWhatSindriDoesNotOwnSaysSo(t *testing.T) {
	e, c, ps := plannerEngine(t, "gh-9", "approved")
	var out bytes.Buffer
	code, err := e.CmdEditTask(c, []string{"gh-9", "--body", "rewritten"}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 || !strings.Contains(out.String(), "nothing changed") {
		t.Errorf("expected a plain report that nothing moved, got exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "--parent") {
		t.Errorf("the reply should name what sindri does own for such a task:\n%s", out.String())
	}
	if got, _ := ps.GetApproval("gh-9"); got != "approved" {
		t.Errorf("an edit that changed nothing must not spend the user's verdict, got %q", got)
	}
}

// TestEditTaskUsageIsVisible: a malformed call answers with usage on the agent's own stream.
// A returned error would reach it as "the hub hit an internal error", which it cannot act on.
func TestEditTaskUsageIsVisible(t *testing.T) {
	e, c, _ := plannerEngine(t, "td-1", "pending")
	for _, args := range [][]string{{}, {"td-1"}, {"td-1", "--nope", "x"}} {
		var out bytes.Buffer
		code, err := e.CmdEditTask(c, args, &out)
		if err != nil {
			t.Fatalf("args %v: usage must not be an error: %v", args, err)
		}
		if code != 2 {
			t.Errorf("args %v: exit = %d, want 2", args, code)
		}
		if !strings.Contains(out.String(), "usage: edit-task") {
			t.Errorf("args %v: expected usage, got: %s", args, out.String())
		}
	}
}

// TestPlannerProposesAPriority replaces the rule that a planner may set none at all. That rule
// conflated two things: a priority is intent and ORDER, approval is authorisation, and only the
// second releases work. A proposal is created pending, so a rating on it can never make it
// claimable — which is why the planner may express the sequence it planned.
func TestPlannerProposesAPriority(t *testing.T) {
	for _, flag := range []string{"--priority", "-p"} {
		spec, words, err := parseTaskFlags([]string{flag, "high", "urgent thing"})
		if err != nil {
			t.Fatalf("%s should be accepted: %v", flag, err)
		}
		if spec.Priority != "P1" {
			t.Errorf("%s high should parse to P1, got %q", flag, spec.Priority)
		}
		if strings.Join(words, " ") != "urgent thing" {
			t.Errorf("%s consumed the title: %v", flag, words)
		}
	}
	// An unrecognised word is refused rather than guessed at — the nearest wrong guess silently
	// re-orders the backlog.
	if _, _, err := parseTaskFlags([]string{"--priority", "urgentish", "a thing"}); err == nil {
		t.Error("an unknown priority should be refused")
	}
	if !strings.Contains(createTaskUsage, "--priority") {
		t.Error("create-task usage should advertise --priority now that it exists")
	}
	// And the usage must no longer say prioritisation is a human decision, which is the half of the
	// old rule that changed.
	if strings.Contains(createTaskUsage, "two human decisions") {
		t.Error("the usage still describes prioritisation as the user's alone")
	}
}

// TestPlannerProposesATier: create-task accepts a difficulty estimate, refusing an unrecognised
// word rather than guessing — the same rule --priority follows.
func TestPlannerProposesATier(t *testing.T) {
	for _, flag := range []string{"--tier", "-T"} {
		spec, words, err := parseTaskFlags([]string{flag, "senior", "hard thing"})
		if err != nil {
			t.Fatalf("%s should be accepted: %v", flag, err)
		}
		if spec.Tier != "senior" {
			t.Errorf("%s senior should parse to senior, got %q", flag, spec.Tier)
		}
		if strings.Join(words, " ") != "hard thing" {
			t.Errorf("%s consumed the title: %v", flag, words)
		}
	}
	if _, _, err := parseTaskFlags([]string{"--tier", "expert", "a thing"}); err == nil {
		t.Error("an unknown tier should be refused")
	}
	if !strings.Contains(createTaskUsage, "--tier") {
		t.Error("create-task usage should advertise --tier now that it exists")
	}
}

// TestEditTaskChangesTierAndGoesBackForAVerdict: unlike priority, tier carries no claimability
// consequence, so edit-task changes it directly rather than refusing the way it refuses --priority
// — and the change must be detected as one, or it reads as "nothing changed" and never returns to
// the user for a fresh look.
func TestEditTaskChangesTierAndGoesBackForAVerdict(t *testing.T) {
	e, c, ps, id, _ := plannerOwnedTask(t, "approved")
	var out bytes.Buffer
	code, err := e.CmdEditTask(c, []string{id, "--tier", "senior"}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("the tier edit should be allowed, got exit %d: %s", code, out.String())
	}
	if strings.Contains(out.String(), "nothing changed") {
		t.Errorf("a tier-only edit read as no change at all: %s", out.String())
	}
	if tk, _, _ := ps.GetTask(id); tk.Tier != "senior" {
		t.Errorf("the edit did not land, tier is %q", tk.Tier)
	}
	if got, _ := ps.GetApproval(id); got != "pending" {
		t.Errorf("after a tier edit the task should await a fresh verdict, got %q", got)
	}
}

// TestUnknownFlagRefused: a silently ignored flag looks like it took effect, and the task is
// then created or edited without the parent or body that was asked for.
func TestUnknownFlagRefused(t *testing.T) {
	if _, _, err := parseTaskFlags([]string{"--tree"}); err == nil {
		t.Error("an unknown flag should be an error the caller sees")
	}
	if _, _, err := parseTaskFlags([]string{"--parent"}); err == nil {
		t.Error("a flag with no value should be an error")
	}
}

// TestParseTaskFlagsFormsAndTitle: both `--flag value` and `--flag=value`, with the leftover
// words forming the title.
func TestParseTaskFlagsFormsAndTitle(t *testing.T) {
	s, words, err := parseTaskFlags([]string{"--parent=os-adc678", "--type", "feature", "--body", "why it matters", "wire", "the", "thing"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Parent != "os-adc678" || s.Type != "feature" || s.Description != "why it matters" {
		t.Errorf("flags parsed as %+v", s)
	}
	if got := strings.Join(words, " "); got != "wire the thing" {
		t.Errorf("title words = %q", got)
	}
}

// TestSyncToleratesRepoWithoutTd: a repo that tracks work only in openspec or GitHub issues
// must still list those. td gates on having a store, so its absence leaves it with nothing to
// contribute instead of failing the sync for every source — which is what made the host CLI
// error with "no td store" while the TUI, reading the cache, happily showed the same repo's
// tasks. One source's absence must not decide the answer for all of them.
func TestSyncToleratesRepoWithoutTd(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir() // no .todos, no openspec/, no GitHub remote
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	e := New(st, &stubDeps{root: root})

	if err := e.SyncTasks("proj"); err != nil {
		t.Fatalf("a repo with no td store should sync cleanly, got: %v", err)
	}
	// And the read path the host CLI uses must agree, rather than surfacing an error the
	// board never sees.
	if _, err := e.Tasks("proj"); err != nil {
		t.Fatalf("Tasks should succeed with no td store, got: %v", err)
	}
}

// workerEngine seeds a worker holding `held` (a package when it has children) and returns the
// engine plus the caller a worker arrives as.
func workerEngine(t *testing.T, tasks []store.Task, container, current string) (*Engine, registry.Caller) {
	t.Helper()
	return workerEngineComments(t, tasks, container, current, nil)
}

// workerEngineComments is workerEngine with a seeded comment thread per task id — the thread lives
// outside the task row, so it is served by deps rather than upserted with the task.
func workerEngineComments(t *testing.T, tasks []store.Task, container, current string, comments map[string][]store.Comment) (*Engine, registry.Caller) {
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
	ps := st.For("proj")
	for _, task := range tasks {
		if err := ps.UpsertTask(task); err != nil {
			t.Fatalf("upsert %s: %v", task.ID, err)
		}
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Container: container, Task: current, Phase: "working"}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	return New(st, &stubDeps{root: root, comments: comments}), registry.Caller{Project: "proj", Agent: "eitri", Role: "worker"}
}

// TestWorkerSeesItsPackage: a package is claimed whole for the context it carries, so that
// context must be re-readable — the claim directive alone loses it to a compaction or relaunch.
// The overview names the parent, its description, and every subtask, marking the current one.
func TestWorkerSeesItsPackage(t *testing.T) {
	e, c := workerEngine(t, []store.Task{
		{ID: "td-EPIC", Title: "Login feature", Status: "open", Priority: "P1", Description: "Users must sign in."},
		{ID: "td-1", Title: "Form UI", Status: "closed", Priority: "P1", ParentID: "td-EPIC"},
		{ID: "td-2", Title: "Session store", Status: "open", Priority: "P1", ParentID: "td-EPIC"},
		{ID: "td-3", Title: "Wire it up", Status: "open", Priority: "P2", ParentID: "td-2"}, // nested
		{ID: "td-OTHER", Title: "Someone else's", Status: "open", Priority: "P1"},
	}, "td-EPIC", "td-2")

	// The view directly: CmdTasks would first sync, and a temp repo has no task source, so
	// the cache would be replaced with nothing before the view ran.
	var out bytes.Buffer
	tasks, err := e.store.For("proj").AllTasks()
	if err != nil {
		t.Fatal(err)
	}
	if code, err := e.workerTaskView(c, tasks, &out); err != nil || code != 0 {
		t.Fatalf("workerTaskView: code=%d err=%v", code, err)
	}
	got := out.String()
	for _, want := range []string{
		"td-EPIC", "Login feature", "Users must sign in.", // the parent and its body
		"td-1", "td-2", "td-3", // every descendant, including the nested one
		"→ td-2",             // the subtask it is on
		"`sindri task <id>`", // how to read one in full
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview missing %q:\n%s", want, got)
		}
	}
	// The overview is its OWN package, not the backlog.
	if strings.Contains(got, "td-OTHER") {
		t.Errorf("a worker's view must not list other work:\n%s", got)
	}
}

// TestWorkerStandaloneTaskShowsInFull: with nothing to choose between, asking the worker to
// pick would be a wasted round trip.
func TestWorkerStandaloneTaskShowsInFull(t *testing.T) {
	e, c := workerEngine(t, []store.Task{
		{ID: "td-9", Title: "Fix the glitch", Status: "open", Priority: "P1", Description: "Steps: reproduce, then fix."},
	}, "", "td-9")

	var out bytes.Buffer
	tasks, err := e.store.For("proj").AllTasks()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.workerTaskView(c, tasks, &out); err != nil {
		t.Fatalf("workerTaskView: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Steps: reproduce, then fix.") {
		t.Errorf("a standalone task should print its description:\n%s", got)
	}
	if strings.Contains(got, "subtasks") {
		t.Errorf("a standalone task has no subtask overview:\n%s", got)
	}
}

// TestWorkerWithNoTaskIsToldWhatToDo: an idle worker gets the one next step, not an empty view.
func TestWorkerWithNoTaskIsToldWhatToDo(t *testing.T) {
	e, c := workerEngine(t, nil, "", "")
	var out bytes.Buffer
	if _, err := e.CmdTasks(c, nil, &out); err != nil {
		t.Fatalf("CmdTasks: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "no task") || !strings.Contains(got, "`sindri`") {
		t.Errorf("expected a pointer to picking work up, got: %q", got)
	}
}

// visibleFor is the boundary under test: which tasks a caller may read, and whether that set is
// bounded at all. Exercised directly rather than through CmdTasks, which syncs first — and a
// temp repo has no task source, so the sync would replace the seeded cache with nothing.
func visibleFor(t *testing.T, e *Engine, c registry.Caller, tasks []store.Task) (map[string]bool, bool) {
	t.Helper()
	v, bounded, err := e.visibleTasks(c, tasks)
	if err != nil {
		t.Fatal(err)
	}
	return v, bounded
}

// TestWorkerIsBoundedToItsPackage: a worker's job is to finish one piece of work, so the rest of
// the backlog is at best a distraction and at worst an invitation to start something nobody
// assigned it. Its package — the held task and every descendant — is what it may read.
func TestWorkerIsBoundedToItsPackage(t *testing.T) {
	tasks := []store.Task{
		{ID: "td-EPIC", Title: "Login feature", Status: "open", Priority: "P1"},
		{ID: "td-1", Title: "Form UI", Status: "open", Priority: "P1", ParentID: "td-EPIC"},
		{ID: "td-2", Title: "Session store", Status: "open", Priority: "P1", ParentID: "td-1"}, // nested
		{ID: "td-OTHER", Title: "Someone else's epic", Status: "open", Priority: "P0"},
		{ID: "td-OC", Title: "…and its child", Status: "open", Priority: "P0", ParentID: "td-OTHER"},
	}
	e, c := workerEngine(t, tasks, "td-EPIC", "td-1")

	visible, bounded := visibleFor(t, e, c, tasks)
	if !bounded {
		t.Fatal("a worker's view must be bounded")
	}
	for _, want := range []string{"td-EPIC", "td-1", "td-2"} { // the package, descendants included
		if !visible[want] {
			t.Errorf("%s is part of the worker's package and must be visible", want)
		}
	}
	for _, hidden := range []string{"td-OTHER", "td-OC"} {
		if visible[hidden] {
			t.Errorf("%s is not the worker's work and must be hidden", hidden)
		}
	}
}

// TestWorkerWithoutTaskSeesNothing: an idle worker has no package, so it reads none of the
// backlog — the bound is on what it HOLDS, not on having asked politely.
func TestWorkerWithoutTaskSeesNothing(t *testing.T) {
	tasks := []store.Task{{ID: "td-A", Title: "Alpha", Status: "open", Priority: "P1"}}
	e, c := workerEngine(t, tasks, "", "")
	visible, bounded := visibleFor(t, e, c, tasks)
	if !bounded || len(visible) != 0 {
		t.Errorf("an idle worker should see nothing, got bounded=%v %v", bounded, visible)
	}
}

// TestPlannerAndCoauthorAreUnbounded: they shape the backlog across packages, and a coauthor
// works directly with the user, so both need the picture the user has.
func TestPlannerAndCoauthorAreUnbounded(t *testing.T) {
	tasks := []store.Task{{ID: "td-A", Status: "open"}, {ID: "td-B", Status: "open"}}
	for _, role := range []string{"planner", "coauthor"} {
		e, c := workerEngine(t, tasks, "", "")
		c.Role = role
		if _, bounded := visibleFor(t, e, c, tasks); bounded {
			t.Errorf("%s must see the whole backlog", role)
		}
	}
}

// TestWorkerIdRefusalNamesTheWayBack: bounding the listing would be theatre if any id could be
// read directly, so the same boundary governs `task <id>` — and the refusal points at the view
// the worker actually wanted, without leaking the task it refused.
func TestWorkerIdRefusalNamesTheWayBack(t *testing.T) {
	tasks := []store.Task{
		{ID: "td-MINE", Title: "Mine", Status: "open", Priority: "P1"},
		{ID: "td-THEIRS", Title: "Theirs", Status: "open", Priority: "P1", Description: "secret plan"},
	}
	e, c := workerEngine(t, tasks, "", "td-MINE")

	var out bytes.Buffer
	code, err := e.CmdTasks(c, []string{"td-THEIRS"}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code == 0 {
		t.Errorf("reading another agent's task must be refused, got exit 0:\n%s", out.String())
	}
	if strings.Contains(out.String(), "secret plan") {
		t.Errorf("the refusal leaked the task body:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "`sindri task`") {
		t.Errorf("the refusal should point at the worker's own package:\n%s", out.String())
	}
}
