package store

import "testing"

func TestTaskCacheAndPriorityOrder(t *testing.T) {
	s := openTmp(t)
	if err := s.ReplaceTasks([]Task{
		{ID: "td-p3", Status: "open", Priority: "P3"},
		{ID: "td-p1", Status: "open", Priority: "P1"},
		{ID: "td-none", Status: "open", Priority: ""}, // unset → sorts last
		{ID: "td-p2", Status: "open", Priority: "P2"},
		{ID: "td-done", Status: "closed", Priority: "P1"},
		{ID: "td-active", Status: "in_progress", Priority: "P1"},
	}); err != nil {
		t.Fatal(err)
	}
	open, err := s.OpenTasks()
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

func TestReplaceTasksMirrors(t *testing.T) {
	s := openTmp(t)
	s.ReplaceTasks([]Task{{ID: "a", Status: "open"}, {ID: "b", Status: "open"}})
	s.ReplaceTasks([]Task{{ID: "b", Status: "open"}}) // a removed
	open, _ := s.OpenTasks()
	if len(open) != 1 || open[0].ID != "b" {
		t.Fatalf("replace did not mirror: %+v", open)
	}
}

func TestAgentStateRoundTrip(t *testing.T) {
	s := openTmp(t)
	// Absent → idle default.
	st, err := s.GetState("brokkr")
	if err != nil || st.Phase != "idle" {
		t.Fatalf("default state: %+v err=%v", st, err)
	}
	if err := s.SetState(AgentState{Agent: "brokkr", Task: "td-1", Branch: "td-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	st, _ = s.GetState("brokkr")
	if st.Task != "td-1" || st.Phase != "working" {
		t.Fatalf("state not persisted: %+v", st)
	}
	// Back to idle clears phase.
	s.SetState(AgentState{Agent: "brokkr", Phase: "idle"})
	st, _ = s.GetState("brokkr")
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

func TestOpenLeavesAndChildren(t *testing.T) {
	s := openTmp(t)
	// A container P with two open children; a standalone leaf L.
	if err := s.ReplaceTasks([]Task{
		{ID: "P", Status: "open", Priority: "P1"},
		{ID: "C1", Status: "open", Priority: "P1", ParentID: "P"},
		{ID: "C2", Status: "open", Priority: "P2", ParentID: "P"},
		{ID: "L", Status: "open", Priority: "P3"},
	}); err != nil {
		t.Fatal(err)
	}

	// Leaves exclude the parent P; children are leaves until reserved.
	if got := ids(mustLeaves(t, s)); !eq(got, []string{"C1", "C2", "L"}) {
		t.Fatalf("OpenLeaves before reservation: got %v", got)
	}
	// Children of a container are its subtask stream regardless of reservation.
	if got := ids(mustChildren(t, s, "P")); !eq(got, []string{"C1", "C2"}) {
		t.Fatalf("OpenChildren: got %v", got)
	}

	// Once an agent holds P, its children are reserved out of the leaf pool.
	if err := s.SetState(AgentState{Agent: "brokkr", Container: "P", Branch: "P", Task: "C1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustLeaves(t, s)); !eq(got, []string{"L"}) {
		t.Fatalf("OpenLeaves after reserving P: got %v", got)
	}
	if got := ids(mustChildren(t, s, "P")); !eq(got, []string{"C1", "C2"}) {
		t.Fatalf("OpenChildren still serves the holder: got %v", got)
	}
}

func TestMarkedContainersAndGetTask(t *testing.T) {
	s := openTmp(t)
	if err := s.ReplaceTasks([]Task{
		{ID: "P", Status: "open", Priority: "P1", Labels: "feature,collab"},     // marked, has open child
		{ID: "C1", Status: "open", Priority: "P1", ParentID: "P"},                // open child of P
		{ID: "Q", Status: "open", Priority: "P1", Labels: "collab"},              // marked but no children
		{ID: "R", Status: "open", Priority: "P1", Labels: "other", ParentID: ""}, // not marked
		{ID: "RC", Status: "open", Priority: "P1", ParentID: "R"},                // R has a child but isn't marked
	}); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustMarked(t, s)); !eq(got, []string{"P"}) {
		t.Fatalf("MarkedContainers: want [P] (marked + has open child + unheld), got %v", got)
	}
	// Holding P removes it from the candidates.
	if err := s.SetState(AgentState{Agent: "brokkr", Container: "P", Branch: "P", Task: "C1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustMarked(t, s)); len(got) != 0 {
		t.Fatalf("a held container must drop out of candidates, got %v", got)
	}

	tk, ok, err := s.GetTask("C1")
	if err != nil || !ok || tk.ParentID != "P" {
		t.Fatalf("GetTask(C1): ok=%v parent=%q err=%v", ok, tk.ParentID, err)
	}
	if _, ok, _ := s.GetTask("nope"); ok {
		t.Fatal("GetTask of a missing id must report ok=false")
	}
}

func mustMarked(t *testing.T, s *Store) []Task {
	t.Helper()
	v, err := s.MarkedContainers("collab")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestAgentStateContainerRoundTrip(t *testing.T) {
	s := openTmp(t)
	if err := s.SetState(AgentState{Agent: "brokkr", Container: "P", Branch: "P", Task: "C1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	st, _ := s.GetState("brokkr")
	if st.Container != "P" || st.Branch != "P" || st.Task != "C1" {
		t.Fatalf("container state not persisted: %+v", st)
	}
}

func mustLeaves(t *testing.T, s *Store) []Task {
	t.Helper()
	v, err := s.OpenLeaves()
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func mustChildren(t *testing.T, s *Store, parent string) []Task {
	t.Helper()
	v, err := s.OpenChildren(parent)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestPRLifecycle(t *testing.T) {
	s := openTmp(t)
	if err := s.PutPR(PR{ID: "pr-td-1", Task: "td-1", Agent: "brokkr", Branch: "td-1", Base: "master"}); err != nil {
		t.Fatal(err)
	}
	pr, ok, err := s.GetPR("pr-td-1")
	if err != nil || !ok {
		t.Fatalf("get pr: ok=%v err=%v", ok, err)
	}
	if pr.Status != "open" || pr.Agent != "brokkr" {
		t.Fatalf("pr defaults wrong: %+v", pr)
	}

	// Status filter.
	s.PutPR(PR{ID: "pr-td-2", Task: "td-2", Status: "merged"})
	openPRs, _ := s.PRs("open")
	if len(openPRs) != 1 || openPRs[0].ID != "pr-td-1" {
		t.Fatalf("open filter wrong: %+v", openPRs)
	}
	all, _ := s.PRs()
	if len(all) != 2 {
		t.Fatalf("all PRs: %d", len(all))
	}

	// Approve then mark merged.
	pr.Status = "approved"
	s.PutPR(pr)
	got, _, _ := s.GetPR("pr-td-1")
	if got.Status != "approved" {
		t.Fatalf("approve not persisted: %+v", got)
	}
}
