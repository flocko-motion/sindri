package tui

import "testing"

// TestResolveAutoParent pins the auto-child rule from td-488d11. The point of
// the rule is that the user navigates to the umbrella, presses n, and the new
// item automatically lands under it — without having to drop into move mode.
func TestResolveAutoParent(t *testing.T) {
	cases := []struct {
		name       string
		parentID   string
		parentType string
		newType    string
		want       string
	}{
		// Cursor on epic: any non-epic new task gets attached.
		{"epic + task → child", "td-eee", "epic", "task", "td-eee"},
		{"epic + bug → child", "td-eee", "epic", "bug", "td-eee"},
		{"epic + feature → child (feature lives under epic)", "td-eee", "epic", "feature", "td-eee"},
		{"epic + chore → child", "td-eee", "epic", "chore", "td-eee"},
		{"epic + epic → no auto-parent (epics are roots)", "td-eee", "epic", "epic", ""},

		// Cursor on feature: only non-epic, non-feature types attach.
		{"feature + task → child", "td-fff", "feature", "task", "td-fff"},
		{"feature + bug → child", "td-fff", "feature", "bug", "td-fff"},
		{"feature + chore → child", "td-fff", "feature", "chore", "td-fff"},
		{"feature + feature → no auto-parent (siblings)", "td-fff", "feature", "feature", ""},
		{"feature + epic → no auto-parent (epic > feature)", "td-fff", "feature", "epic", ""},

		// Cursor on anything else: no auto-parent.
		{"task + task → no auto-parent", "td-aaa", "task", "task", ""},
		{"bug + bug → no auto-parent", "td-aaa", "bug", "bug", ""},
		{"chore + anything → no auto-parent", "td-aaa", "chore", "task", ""},

		// Empty parent → empty result (no row eligible).
		{"empty parent → no auto-parent", "", "", "task", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveAutoParent(c.parentID, c.parentType, c.newType); got != c.want {
				t.Errorf("resolveAutoParent(%q, %q, %q) = %q, want %q",
					c.parentID, c.parentType, c.newType, got, c.want)
			}
		})
	}
}
