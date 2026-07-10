package store

import "testing"

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

func TestAgentStateRoundTrip(t *testing.T) {
	p := openTmpProject(t)
	// Absent → idle default.
	st, err := p.GetState("brokkr")
	if err != nil || st.Phase != "idle" {
		t.Fatalf("default state: %+v err=%v", st, err)
	}
	if err := p.SetState(AgentState{Agent: "brokkr", Task: "td-1", Branch: "td-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	st, _ = p.GetState("brokkr")
	if st.Task != "td-1" || st.Phase != "working" {
		t.Fatalf("state not persisted: %+v", st)
	}
	// Back to idle clears phase.
	p.SetState(AgentState{Agent: "brokkr", Phase: "idle"})
	st, _ = p.GetState("brokkr")
	if st.Phase != "idle" || st.Task != "" {
		t.Fatalf("idle not applied: %+v", st)
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
	if err := p.SetState(AgentState{Agent: "eitri", Task: "gh-7", Branch: "gh-7", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustLeaves(t, p)); !eq(got, []string{"td-p1"}) {
		t.Fatalf("held gh-7 must be excluded from leaves: got %v", got)
	}
}

func TestOpenLeavesAndChildren(t *testing.T) {
	p := openTmpProject(t)
	// A container P with two open children; a standalone leaf L.
	if err := p.ReplaceTasks([]Task{
		{ID: "P", Status: "open", Priority: "P1"},
		{ID: "C1", Status: "open", Priority: "P1", ParentID: "P"},
		{ID: "C2", Status: "open", Priority: "P2", ParentID: "P"},
		{ID: "L", Status: "open", Priority: "P3"},
	}); err != nil {
		t.Fatal(err)
	}

	// Leaves exclude the parent P; children are leaves until reserved.
	if got := ids(mustLeaves(t, p)); !eq(got, []string{"C1", "C2", "L"}) {
		t.Fatalf("OpenLeaves before reservation: got %v", got)
	}
	// Children of a container are its subtask stream regardless of reservation.
	if got := ids(mustChildren(t, p, "P")); !eq(got, []string{"C1", "C2"}) {
		t.Fatalf("OpenChildren: got %v", got)
	}

	// Once an agent holds P, its children are reserved out of the leaf pool.
	if err := p.SetState(AgentState{Agent: "brokkr", Container: "P", Branch: "P", Task: "C1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustLeaves(t, p)); !eq(got, []string{"L"}) {
		t.Fatalf("OpenLeaves after reserving P: got %v", got)
	}
	if got := ids(mustChildren(t, p, "P")); !eq(got, []string{"C1", "C2"}) {
		t.Fatalf("OpenChildren still serves the holder: got %v", got)
	}
}

func TestMarkedContainersAndGetTask(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "P", Status: "open", Priority: "P1", Labels: "feature,collab"},     // marked, has open child
		{ID: "C1", Status: "open", Priority: "P1", ParentID: "P"},                // open child of P
		{ID: "Q", Status: "open", Priority: "P1", Labels: "collab"},              // marked but no children
		{ID: "R", Status: "open", Priority: "P1", Labels: "other", ParentID: ""}, // not marked
		{ID: "RC", Status: "open", Priority: "P1", ParentID: "R"},                // R has a child but isn't marked
	}); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustMarked(t, p)); !eq(got, []string{"P"}) {
		t.Fatalf("MarkedContainers: want [P] (marked + has open child + unheld), got %v", got)
	}
	// Holding P removes it from the candidates.
	if err := p.SetState(AgentState{Agent: "brokkr", Container: "P", Branch: "P", Task: "C1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustMarked(t, p)); len(got) != 0 {
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

func mustMarked(t *testing.T, p *ProjectStore) []Task {
	t.Helper()
	v, err := p.MarkedContainers("collab")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestAgentStateContainerRoundTrip(t *testing.T) {
	p := openTmpProject(t)
	if err := p.SetState(AgentState{Agent: "brokkr", Container: "P", Branch: "P", Task: "C1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	st, _ := p.GetState("brokkr")
	if st.Container != "P" || st.Branch != "P" || st.Task != "C1" {
		t.Fatalf("container state not persisted: %+v", st)
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

func mustChildren(t *testing.T, p *ProjectStore, parent string) []Task {
	t.Helper()
	v, err := p.OpenChildren(parent)
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
}
