package store

import (
	"testing"
	"time"
)

func openTmpProject(t *testing.T) *ProjectStore { return openTmp(t).For("repo") }

func TestTaskCacheAndPriorityOrder(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "td-p3", Status: "open", Priority: "P3"},
		{ID: "td-p1", Status: "open", Priority: "P1"},
		{ID: "td-none", Status: "open", Priority: ""}, // unset → sorts last
		{ID: "td-p2", Status: "open", Priority: "P2"},
		{ID: "td-done", Status: "closed", Priority: "P1"},
		{ID: "td-active", Status: "in_progress", Priority: "P1"},
	}); err != nil {
		t.Fatal(err)
	}
	open, err := p.OpenTasks()
	if err != nil {
		t.Fatal(err)
	}
	// Only status=open, highest priority (P1) first, unset last; closed and
	// in_progress excluded.
	got := []string{}
	for _, o := range open {
		got = append(got, o.ID)
	}
	want := []string{"td-p1", "td-p2", "td-p3", "td-none"}
	if len(got) != len(want) {
		t.Fatalf("open tasks: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order: got %v want %v", got, want)
		}
	}
}

func TestOpenLeavesRequiresPriority(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "td-p2", Status: "open", Priority: "P2"},
		{ID: "td-none", Status: "open", Priority: ""}, // no priority → not auto-assignable
		{ID: "td-p1", Status: "open", Priority: "P1"},
	}); err != nil {
		t.Fatal(err)
	}

	// No prio, no assignment: the auto-assigner's leaf set excludes the
	// unprioritized task, and returns the rest highest-priority first.
	leaves, err := p.OpenLeaves()
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, l := range leaves {
		got = append(got, l.ID)
	}
	want := []string{"td-p1", "td-p2"}
	if len(got) != len(want) {
		t.Fatalf("open leaves: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("open leaves order/contents: got %v want %v", got, want)
		}
	}

	// It's still in the backlog (OpenTasks) — visible and editable — just not
	// auto-assigned until a human gives it a priority.
	open, _ := p.OpenTasks()
	var seenNone bool
	for _, o := range open {
		if o.ID == "td-none" {
			seenNone = true
		}
	}
	if !seenNone {
		t.Fatalf("unprioritized task should stay visible in the backlog: %+v", open)
	}
}

func TestReplaceTasksMirrors(t *testing.T) {
	p := openTmpProject(t)
	p.ReplaceTasks([]Task{{ID: "a", Status: "open"}, {ID: "b", Status: "open"}})
	p.ReplaceTasks([]Task{{ID: "b", Status: "open"}}) // a removed
	open, _ := p.OpenTasks()
	if len(open) != 1 || open[0].ID != "b" {
		t.Fatalf("replace did not mirror: %+v", open)
	}
}

// TestTaskDescriptionPersists guards the fix: the description (a GitHub issue body)
// must survive ReplaceTasks and come back on GetTask — it used to be dropped because
// the tasks table had no description column.
func TestTaskDescriptionPersists(t *testing.T) {
	p := openTmpProject(t)
	body := "## Steps\n1. reproduce\n2. fix"
	p.ReplaceTasks([]Task{{ID: "gh-42", Status: "open", Type: "issue", Description: body}})
	got, ok, err := p.GetTask("gh-42")
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if got.Description != body {
		t.Fatalf("description not persisted: got %q, want %q", got.Description, body)
	}
}

// TestTaskTierPersists: the difficulty estimate round-trips through both write paths — the bulk
// ReplaceTasks a sync does, and the point UpsertTask a single-task refresh does.
func TestTaskTierPersists(t *testing.T) {
	p := openTmpProject(t)
	p.ReplaceTasks([]Task{{ID: "td-1", Status: "open", Type: "task", Tier: "senior"}})
	got, ok, err := p.GetTask("td-1")
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if got.Tier != "senior" {
		t.Fatalf("tier not persisted via ReplaceTasks: got %q, want senior", got.Tier)
	}
	if err := p.UpsertTask(Task{ID: "td-1", Status: "open", Type: "task", Tier: "junior"}); err != nil {
		t.Fatal(err)
	}
	got, ok, err = p.GetTask("td-1")
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if got.Tier != "junior" {
		t.Fatalf("tier not persisted via UpsertTask: got %q, want junior", got.Tier)
	}
}

// TestTaskURLPersists: a GitHub issue's URL round-trips through both write paths — the bulk
// ReplaceTasks a sync does, and the point UpsertTask a single-task refresh does — and survives
// the read paths (GetTask, AllTasks) a plain task with no URL leaves it "" through either.
func TestTaskURLPersists(t *testing.T) {
	p := openTmpProject(t)
	url := "https://github.com/flo-at/sindri/issues/42"
	if err := p.ReplaceTasks([]Task{
		{ID: "gh-42", Status: "open", Type: "issue", URL: url},
		{ID: "td-1", Status: "open"}, // a plain task never has one
	}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := p.GetTask("gh-42")
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if got.URL != url {
		t.Fatalf("ReplaceTasks: URL not persisted: got %q, want %q", got.URL, url)
	}
	if plain, _, _ := p.GetTask("td-1"); plain.URL != "" {
		t.Errorf("a task with no URL must read back \"\", got %q", plain.URL)
	}

	// UpsertTask (the single-task refresh path) must persist and update it too.
	if err := p.UpsertTask(Task{ID: "gh-42", Status: "open", Type: "issue", URL: url + "?x=1"}); err != nil {
		t.Fatal(err)
	}
	got, _, err = p.GetTask("gh-42")
	if err != nil {
		t.Fatal(err)
	}
	if want := url + "?x=1"; got.URL != want {
		t.Fatalf("UpsertTask: URL not updated: got %q, want %q", got.URL, want)
	}

	all, err := p.AllTasks()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tk := range all {
		if tk.ID == "gh-42" {
			found = true
			if tk.URL == "" {
				t.Error("AllTasks must carry the URL too, not just GetTask")
			}
		}
	}
	if !found {
		t.Fatal("gh-42 missing from AllTasks")
	}
}

// TestTaskUpdatedAtPersists: the "active" tasks filter reads this off the cached row, so both
// write paths (a full sync and the single-task refresh) must carry it through, same as URL above.
func TestTaskUpdatedAtPersists(t *testing.T) {
	p := openTmpProject(t)
	at := "2026-07-30T12:00:00Z"
	if err := p.ReplaceTasks([]Task{
		{ID: "td-1", Status: "closed", UpdatedAt: at},
		{ID: "td-2", Status: "open"}, // no known change time
	}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := p.GetTask("td-1")
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if got.UpdatedAt != at {
		t.Fatalf("ReplaceTasks: UpdatedAt not persisted: got %q, want %q", got.UpdatedAt, at)
	}
	if plain, _, _ := p.GetTask("td-2"); plain.UpdatedAt != "" {
		t.Errorf("a task with no known change time must read back \"\", got %q", plain.UpdatedAt)
	}

	later := "2026-07-30T13:00:00Z"
	if err := p.UpsertTask(Task{ID: "td-1", Status: "closed", UpdatedAt: later}); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := p.GetTask("td-1"); got.UpdatedAt != later {
		t.Fatalf("UpsertTask: UpdatedAt not updated: got %q, want %q", got.UpdatedAt, later)
	}

	all, err := p.AllTasks()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tk := range all {
		if tk.ID == "td-1" {
			found = true
			if tk.UpdatedAt != later {
				t.Error("AllTasks must carry UpdatedAt too, not just GetTask")
			}
		}
	}
	if !found {
		t.Fatal("td-1 missing from AllTasks")
	}
}

func TestAgentStateRoundTrip(t *testing.T) {
	p := openTmpProject(t)
	// Absent → idle default.
	st, err := p.GetState("brokkr")
	if err != nil || st.Phase != "idle" {
		t.Fatalf("default state: %+v err=%v", st, err)
	}
	if err := p.SetState(AgentState{Agent: "brokkr", Task: "td-1", Branch: "td-1", Phase: "working"}, ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	st, _ = p.GetState("brokkr")
	if st.Task != "td-1" || st.Phase != "working" {
		t.Fatalf("state not persisted: %+v", st)
	}
	// Back to idle clears phase.
	p.SetState(AgentState{Agent: "brokkr", Phase: "idle"}, ReasonClaimed, "test setup")
	st, _ = p.GetState("brokkr")
	if st.Phase != "idle" || st.Task != "" {
		t.Fatalf("idle not applied: %+v", st)
	}
}

// TestLastNudgeRoundTrip: SetLastNudge persists, SetState leaves it alone (same reason escalation and
// notes_left are excluded from it), and SetLastNudge("") clears it once a claim ends the memory.
func TestLastNudgeRoundTrip(t *testing.T) {
	p := openTmpProject(t)
	st, err := p.GetState("brokkr")
	if err != nil || st.LastNudge != "" {
		t.Fatalf("default last_nudge: %+v err=%v", st, err)
	}
	if err := p.SetLastNudge("brokkr", "td-1"); err != nil {
		t.Fatal(err)
	}
	if st, _ = p.GetState("brokkr"); st.LastNudge != "td-1" {
		t.Fatalf("last_nudge not persisted: %+v", st)
	}
	// A phase change is not a claim — the memory must survive it.
	if err := p.SetState(AgentState{Agent: "brokkr", Phase: "working"}, ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	if st, _ = p.GetState("brokkr"); st.LastNudge != "td-1" {
		t.Fatalf("SetState must leave last_nudge alone: %+v", st)
	}
	if err := p.SetLastNudge("brokkr", ""); err != nil {
		t.Fatal(err)
	}
	if st, _ = p.GetState("brokkr"); st.LastNudge != "" {
		t.Fatalf("last_nudge not cleared: %+v", st)
	}
}

func ids(tasks []Task) []string {
	out := []string{}
	for _, t := range tasks {
		out = append(out, t.ID)
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestOpenLeavesExcludesHeldLeaf: a gh-* issue rides the store as an open, childless
// P4 leaf (so it's directly claimable), but once an agent holds it (agent_state.task)
// it drops out of the leaf pool — the mechanism that stops a claimed GitHub issue from
// being handed out twice, since GitHub keeps the issue "open" and there's no source
// status to flip (unlike a td task going in_progress).
func TestOpenLeavesExcludesHeldLeaf(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "gh-7", Status: "open", Priority: "P4", Type: "issue"},
		{ID: "td-p1", Status: "open", Priority: "P1"},
	}); err != nil {
		t.Fatal(err)
	}

	// The gh issue is a claimable leaf alongside the td task.
	if got := ids(mustLeaves(t, p)); !eq(got, []string{"td-p1", "gh-7"}) {
		t.Fatalf("gh-* leaf should be claimable: got %v", got)
	}

	// Once a worker holds gh-7, it leaves the pool (no source status flip to rely on).
	if err := p.SetState(AgentState{Agent: "eitri", Task: "gh-7", Branch: "gh-7", Phase: "working"}, ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustLeaves(t, p)); !eq(got, []string{"td-p1"}) {
		t.Fatalf("held gh-7 must be excluded from leaves: got %v", got)
	}
}

func TestOpenLeavesAndChildren(t *testing.T) {
	p := openTmpProject(t)
	// A package P with two open children; a standalone leaf L.
	if err := p.ReplaceTasks([]Task{
		{ID: "P", Status: "open", Priority: "P1"},
		{ID: "C1", Status: "open", Priority: "P1", ParentID: "P"},
		{ID: "C2", Status: "open", Priority: "P2", ParentID: "P"},
		{ID: "L", Status: "open", Priority: "P3"},
	}); err != nil {
		t.Fatal(err)
	}

	// The leaf pool is standalone tasks only: P is a package (-> OpenContainers) and its
	// children are worked inside it, so L is the whole pool — before any reservation.
	if got := ids(mustLeaves(t, p)); !eq(got, []string{"L"}) {
		t.Fatalf("OpenLeaves: want [L] (standalone only), got %v", got)
	}
	// Children remain the package's subtask stream, which is how the holder works them.
	if got := ids(mustSubtasks(t, p, "P")); !eq(got, []string{"C1", "C2"}) {
		t.Fatalf("OpenSubtasks: got %v", got)
	}

	// Holding P changes nothing for the leaf pool: its children were never in it.
	if err := p.SetState(AgentState{Agent: "brokkr", Container: "P", Branch: "P", Task: "C1", Phase: "working"}, ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustLeaves(t, p)); !eq(got, []string{"L"}) {
		t.Fatalf("OpenLeaves after reserving P: got %v", got)
	}
	if got := ids(mustSubtasks(t, p, "P")); !eq(got, []string{"C1", "C2"}) {
		t.Fatalf("OpenSubtasks still serves the holder: got %v", got)
	}
}

// TestOpenContainersAndGetTask: every parent with an open child is a package — a hierarchy
// is organised that way so one agent takes the whole thing, so no label is needed to opt in.
// A package needs the same release gates as a standalone task (approved, prioritised), and
// the topmost open ancestor is the one claimed.
func TestOpenContainersAndGetTask(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "P", Status: "open", Priority: "P1", Labels: "feature"}, // has an open child
		{ID: "C1", Status: "open", Priority: "P1", ParentID: "P"},    // its child
		{ID: "Q", Status: "open", Priority: "P1"},                    // no children — a standalone task
		{ID: "N", Status: "open"},                                    // has a child but NO priority
		{ID: "NC", Status: "open", Priority: "P1", ParentID: "N"},
		{ID: "G", Status: "open", Priority: "P1"}, // grandparent
		{ID: "GP", Status: "open", Priority: "P1", ParentID: "G"},
		{ID: "GC", Status: "open", Priority: "P1", ParentID: "GP"},
	}); err != nil {
		t.Fatal(err)
	}
	// P is a package; G is one too, but GP is inside G so it isn't claimed separately. N has
	// no priority, so nothing about it is released yet.
	if got := ids(mustContainers(t, p)); !eq(got, []string{"G", "P"}) {
		t.Fatalf("OpenContainers: want [G P], got %v", got)
	}
	// Holding P removes it from the candidates.
	if err := p.SetState(AgentState{Agent: "brokkr", Container: "P", Branch: "P", Task: "C1", Phase: "working"}, ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustContainers(t, p)); !eq(got, []string{"G"}) {
		t.Fatalf("a held container must drop out of candidates, got %v", got)
	}

	tk, ok, err := p.GetTask("C1")
	if err != nil || !ok || tk.ParentID != "P" {
		t.Fatalf("GetTask(C1): ok=%v parent=%q err=%v", ok, tk.ParentID, err)
	}
	if _, ok, _ := p.GetTask("nope"); ok {
		t.Fatal("GetTask of a missing id must report ok=false")
	}
}

// TestOpenLeavesLeavesPackagesAlone: a child is worked inside its package, on the package's
// branch, so it must never be handed out on its own — that would split one tree across
// agents and strip the context the hierarchy was built to provide.
func TestOpenLeavesLeavesPackagesAlone(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "P", Status: "open", Priority: "P1"},
		{ID: "C1", Status: "open", Priority: "P0", ParentID: "P"}, // higher priority, still not standalone
		{ID: "C2", Status: "open", Priority: "P1", ParentID: "P"},
		{ID: "S", Status: "open", Priority: "P2"},                 // standalone
		{ID: "D", Status: "closed", Priority: "P1"},               // done parent…
		{ID: "DC", Status: "open", Priority: "P1", ParentID: "D"}, // …so its child stands alone
	}); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustLeaves(t, p)); !eq(got, []string{"DC", "S"}) {
		t.Fatalf("OpenLeaves: want [DC S] (standalone only), got %v", got)
	}
}

func mustContainers(t *testing.T, p *ProjectStore) []Task {
	t.Helper()
	v, err := p.OpenContainers()
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestAgentStateContainerRoundTrip(t *testing.T) {
	p := openTmpProject(t)
	if err := p.SetState(AgentState{Agent: "brokkr", Container: "P", Branch: "P", Task: "C1", Phase: "working"}, ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	st, _ := p.GetState("brokkr")
	if st.Container != "P" || st.Branch != "P" || st.Task != "C1" {
		t.Fatalf("container state not persisted: %+v", st)
	}
}

func TestSetPhaseLeavesTaskBranchAndContainerAlone(t *testing.T) {
	p := openTmpProject(t)
	if err := p.SetState(AgentState{Agent: "brokkr", Container: "P", Branch: "P", Task: "C1", Phase: "working"}, ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetPhase("brokkr", "resolving", ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	st, _ := p.GetState("brokkr")
	if st.Container != "P" || st.Branch != "P" || st.Task != "C1" || st.Phase != "resolving" {
		t.Fatalf("SetPhase must change only phase, got %+v", st)
	}
}

func TestSetPhaseErrorsRatherThanNoOpOnAnUnknownAgent(t *testing.T) {
	p := openTmpProject(t)
	if err := p.SetPhase("nobody", "working", ReasonClaimed, "test setup"); err == nil {
		t.Fatal("SetPhase against an agent with no row must error, not silently do nothing")
	}
}

func mustLeaves(t *testing.T, p *ProjectStore) []Task {
	t.Helper()
	v, err := p.OpenLeaves()
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func mustSubtasks(t *testing.T, p *ProjectStore, parent string) []Task {
	t.Helper()
	v, err := p.OpenSubtasks(parent)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestPRLifecycle(t *testing.T) {
	p := openTmpProject(t)
	if err := p.PutPR(PR{ID: "pr-td-1", Task: "td-1", Agent: "brokkr", Branch: "td-1", Base: "master"}); err != nil {
		t.Fatal(err)
	}
	pr, ok, err := p.GetPR("pr-td-1")
	if err != nil || !ok {
		t.Fatalf("get pr: ok=%v err=%v", ok, err)
	}
	if pr.Status != "open" || pr.Agent != "brokkr" || pr.Project != "repo" {
		t.Fatalf("pr defaults wrong: %+v", pr)
	}
	firstUpdate, err := time.Parse(time.RFC3339, pr.UpdatedAt)
	if err != nil || time.Since(firstUpdate) > time.Minute {
		t.Fatalf("updated_at not stamped on insert: %q (err %v)", pr.UpdatedAt, err)
	}

	// Status filter.
	p.PutPR(PR{ID: "pr-td-2", Task: "td-2", Status: "merged"})
	openPRs, _ := p.PRs("open")
	if len(openPRs) != 1 || openPRs[0].ID != "pr-td-1" {
		t.Fatalf("open filter wrong: %+v", openPRs)
	}
	all, _ := p.PRs()
	if len(all) != 2 {
		t.Fatalf("all PRs: %d", len(all))
	}

	// AllPRs (global board) sees them too, tagged with project.
	if g, _ := p.s.AllPRs(); len(g) != 2 || g[0].Project != "repo" {
		t.Fatalf("AllPRs: %+v", g)
	}

	// Approve then mark merged.
	pr.Status = "approved"
	p.PutPR(pr)
	got, _, _ := p.GetPR("pr-td-1")
	if got.Status != "approved" {
		t.Fatalf("approve not persisted: %+v", got)
	}
	// updated_at is stamped fresh on every write, ignoring whatever the caller's struct carried —
	// pr.UpdatedAt here is still the FIRST timestamp, read back before this second write.
	secondUpdate, err := time.Parse(time.RFC3339, got.UpdatedAt)
	if err != nil || secondUpdate.Before(firstUpdate) {
		t.Fatalf("updated_at not refreshed on the second write: first=%v second=%q (err %v)", firstUpdate, got.UpdatedAt, err)
	}
}

// TestReviewingPR: the store can report which PR a reviewer is currently working
// on — the most recent review assigned to it with no verdict yet — so the board
// can show what a reviewer is reviewing. A finished (verdict-recorded) review no
// longer counts, and an unrelated reviewer sees nothing.
func TestReviewingPR(t *testing.T) {
	p := openTmpProject(t)

	// No reviews yet → nothing under review.
	if pr, err := p.ReviewingPR("dvalin"); err != nil || pr != "" {
		t.Fatalf("empty case: pr=%q err=%v", pr, err)
	}

	// A review assigned to dvalin, still open (no verdict) → that's its PR.
	id, err := p.AddReview("pr-td-1", "check the thing")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AssignReview(id, "dvalin"); err != nil {
		t.Fatal(err)
	}
	if pr, err := p.ReviewingPR("dvalin"); err != nil || pr != "pr-td-1" {
		t.Fatalf("in-progress review: pr=%q err=%v, want pr-td-1", pr, err)
	}
	// A different reviewer isn't reviewing it.
	if pr, _ := p.ReviewingPR("eitri"); pr != "" {
		t.Fatalf("unrelated reviewer should see nothing, got %q", pr)
	}

	// Once a verdict lands, it's no longer "currently reviewing".
	if err := p.RecordVerdict(id, "pass", "looks good"); err != nil {
		t.Fatal(err)
	}
	if pr, _ := p.ReviewingPR("dvalin"); pr != "" {
		t.Fatalf("completed review should not count, got %q", pr)
	}
}

// TestStatusChangedAtMovesOnlyOnAStatusChange is the whole reason the column exists beside
// updated_at, which every write refreshes. A PR rebased onto a new base, or given feedback, is still
// sitting in the status it was in — answering "how long has this been open" from the last write
// would report a week-old PR as an hour old, which is the number a person acts on.
func TestStatusChangedAtMovesOnlyOnAStatusChange(t *testing.T) {
	p := openTmpProject(t)
	pr := PR{ID: "pr-td-9", Task: "td-9", Agent: "dvalin", Branch: "b", Base: "master", Status: "open"}
	if err := p.PutPR(pr); err != nil {
		t.Fatalf("put: %v", err)
	}
	first, _, _ := p.GetPR("pr-td-9")
	if first.StatusChangedAt == "" {
		t.Fatal("a new PR must record when it reached its first status")
	}

	// A write that leaves the status alone: the stamp must stand, however much else moved.
	pr = first
	pr.Base, pr.Feedback = "main", "please fix the naming"
	time.Sleep(1100 * time.Millisecond) // RFC3339 is second-resolution, so a change must cross one
	if err := p.PutPR(pr); err != nil {
		t.Fatalf("put: %v", err)
	}
	held, _, _ := p.GetPR("pr-td-9")
	if held.StatusChangedAt != first.StatusChangedAt {
		t.Errorf("a rebase moved the status stamp: %q -> %q", first.StatusChangedAt, held.StatusChangedAt)
	}
	if held.UpdatedAt == first.UpdatedAt {
		t.Error("updated_at must still move on every write — the two stamps answer different questions")
	}

	// And the transition itself does move it.
	pr = held
	pr.Status = "approved"
	if err := p.PutPR(pr); err != nil {
		t.Fatalf("put: %v", err)
	}
	moved, _, _ := p.GetPR("pr-td-9")
	if moved.StatusChangedAt == first.StatusChangedAt {
		t.Errorf("reaching %q left the stamp at %q", moved.Status, moved.StatusChangedAt)
	}
}
