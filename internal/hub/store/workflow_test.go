package store

import "testing"

func TestTaskCacheAndPriorityOrder(t *testing.T) {
	s := openTmp(t)
	if err := s.ReplaceTasks([]Task{
		{ID: "td-low", Status: "open", Priority: "low"},
		{ID: "td-crit", Status: "open", Priority: "critical"},
		{ID: "td-med", Status: "open", Priority: "medium"},
		{ID: "td-done", Status: "closed", Priority: "high"},
		{ID: "td-active", Status: "in_progress", Priority: "high"},
	}); err != nil {
		t.Fatal(err)
	}
	open, err := s.OpenTasks()
	if err != nil {
		t.Fatal(err)
	}
	// Only status=open, highest priority first; closed and in_progress excluded.
	got := []string{}
	for _, o := range open {
		got = append(got, o.ID)
	}
	want := []string{"td-crit", "td-med", "td-low"}
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
