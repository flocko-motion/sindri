package issue

import "testing"

func TestStateClassification(t *testing.T) {
	cases := []struct {
		status               string
		closed, active, open bool
	}{
		{"open", false, false, true},
		{"in_progress", false, true, false},
		{"in_review", false, true, false},
		{"closed", true, false, false},
		{"approved", true, false, false},
		{"merged", true, false, false},
	}
	for _, c := range cases {
		tk := Task{Status: c.status}
		if tk.IsClosed() != c.closed || tk.IsActive() != c.active || tk.IsOpen() != c.open {
			t.Errorf("%s: closed=%v active=%v open=%v", c.status, tk.IsClosed(), tk.IsActive(), tk.IsOpen())
		}
	}
}

func TestSpecName(t *testing.T) {
	if got := (Task{Labels: []string{"spec:add-auth", "require-review-code"}}).SpecName(); got != "add-auth" {
		t.Errorf("SpecName() = %q want add-auth", got)
	}
	if got := (Task{Labels: []string{"require-review-code"}}).SpecName(); got != "" {
		t.Errorf("SpecName() = %q want empty", got)
	}
}

func TestGates(t *testing.T) {
	tk := Task{Labels: []string{
		"require-review-code",
		"require-review-security",
		"approved-review-code",
	}}
	gates := tk.Gates()
	if len(gates) != 2 {
		t.Fatalf("want 2 gates, got %d", len(gates))
	}
	byName := map[string]bool{}
	for _, g := range gates {
		byName[g.Name] = g.Approved
	}
	// Gate names keep the "review-" portion so they display as "review code".
	if !byName["review-code"] {
		t.Error("review-code gate should be approved")
	}
	if byName["review-security"] {
		t.Error("review-security gate should not be approved")
	}
	missing := tk.MissingReviews()
	if len(missing) != 1 || missing[0] != "review-security" {
		t.Errorf("MissingReviews = %v want [review-security]", missing)
	}
}

func TestTaskIDFromTitle(t *testing.T) {
	cases := map[string]string{
		"fix(td-abc123): something":  "td-abc123",
		"feat(td-0f9b1a): add thing": "td-0f9b1a",
		"no task id here":            "",
		"td-deadbe plain":            "td-deadbe",
	}
	for title, want := range cases {
		if got := TaskIDFromTitle(title); got != want {
			t.Errorf("TaskIDFromTitle(%q) = %q want %q", title, got, want)
		}
	}
}

func TestAssemble(t *testing.T) {
	tasks := []Task{
		{ID: "td-1", Status: "open"},
		{ID: "td-2", Status: "open", Labels: []string{"spec:linked-spec"}},
	}
	specs := []Spec{
		{Name: "linked-spec"},   // has a task → not surfaced alone
		{Name: "orphan-spec"},   // no task → surfaced as spec-only
	}
	issues := Assemble(tasks, specs, map[string]string{"td-1": "brokkr"}, nil)

	// orphan-spec (spec-only) comes first, then the two tasks.
	if len(issues) != 3 {
		t.Fatalf("want 3 issues, got %d", len(issues))
	}
	if !issues[0].SpecOnly() || issues[0].Spec.Name != "orphan-spec" {
		t.Errorf("first issue should be spec-only orphan-spec, got %+v", issues[0])
	}
	if issues[0].ID() != SpecID("orphan-spec") {
		t.Errorf("spec-only ID = %q want %q", issues[0].ID(), SpecID("orphan-spec"))
	}
	// td-2 must carry its linked spec.
	var found bool
	for _, iss := range issues {
		if iss.HasTask() && iss.Task.ID == "td-2" {
			found = true
			if !iss.HasSpec() || iss.Spec.Name != "linked-spec" {
				t.Error("td-2 should carry linked-spec")
			}
		}
	}
	if !found {
		t.Error("td-2 missing from assembled issues")
	}
}
