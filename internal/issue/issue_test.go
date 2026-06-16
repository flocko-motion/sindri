package issue

import "testing"

func TestTaskStateClassification(t *testing.T) {
	cases := []struct {
		status         string
		closed, active bool
	}{
		{"open", false, false},
		{"in_progress", false, true},
		{"in_review", false, true},
		{"closed", true, false},
		{"approved", true, false},
		{"merged", true, false},
	}
	for _, c := range cases {
		tk := Task{Status: c.status}
		if tk.IsClosed() != c.closed {
			t.Errorf("%s: IsClosed=%v want %v", c.status, tk.IsClosed(), c.closed)
		}
		if tk.IsActive() != c.active {
			t.Errorf("%s: IsActive=%v want %v", c.status, tk.IsActive(), c.active)
		}
	}
}
