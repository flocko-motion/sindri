package issue

import "testing"

func TestStateClassification(t *testing.T) {
	cases := []struct {
		status                   string
		closed, active, open     bool
	}{
		{"open", false, false, true},
		{"in_progress", false, true, false},
		{"in_review", false, true, false},
		{"closed", true, false, false},
		{"approved", true, false, false},
		{"merged", true, false, false},
	}
	for _, c := range cases {
		i := Issue{Status: c.status}
		if i.IsClosed() != c.closed || i.IsActive() != c.active || i.IsOpen() != c.open {
			t.Errorf("%s: closed=%v active=%v open=%v", c.status, i.IsClosed(), i.IsActive(), i.IsOpen())
		}
	}
}

func TestSpec(t *testing.T) {
	if got := (Issue{Labels: []string{"spec:add-auth", "require-review-code"}}).Spec(); got != "add-auth" {
		t.Errorf("Spec() = %q want add-auth", got)
	}
	if got := (Issue{Labels: []string{"require-review-code"}}).Spec(); got != "" {
		t.Errorf("Spec() = %q want empty", got)
	}
}

func TestGates(t *testing.T) {
	i := Issue{Labels: []string{
		"require-review-code",
		"require-review-security",
		"approved-review-code",
	}}
	gates := i.Gates()
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
	missing := i.MissingReviews()
	if len(missing) != 1 || missing[0] != "review-security" {
		t.Errorf("MissingReviews = %v want [review-security]", missing)
	}
}

func TestTaskIDFromTitle(t *testing.T) {
	cases := map[string]string{
		"fix(td-abc123): something":     "td-abc123",
		"feat(td-0f9b1a): add thing":    "td-0f9b1a",
		"no task id here":               "",
		"td-deadbe plain":               "td-deadbe",
	}
	for title, want := range cases {
		if got := TaskIDFromTitle(title); got != want {
			t.Errorf("TaskIDFromTitle(%q) = %q want %q", title, got, want)
		}
	}
}

func TestParseAndSort(t *testing.T) {
	data := []byte(`[
		{"id":"td-1","status":"closed","updated_at":"2026-05-27T10:00:00Z"},
		{"id":"td-2","status":"open","priority":"P0"},
		{"id":"td-3","status":"in_progress","updated_at":"2026-05-28T10:00:00Z"},
		{"id":"td-4","status":"in_review","updated_at":"2026-05-28T12:00:00Z"}
	]`)
	got, err := parseAndSort(data)
	if err != nil {
		t.Fatal(err)
	}
	// Expected order: open first, then active (recent first), then closed.
	wantOrder := []string{"td-2", "td-4", "td-3", "td-1"}
	for i, w := range wantOrder {
		if got[i].ID != w {
			t.Errorf("position %d: got %s want %s", i, got[i].ID, w)
		}
	}
}
