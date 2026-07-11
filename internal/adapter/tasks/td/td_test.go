package td

import (
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/task"
)

func TestTdErrorMessage(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			name: "extracts the Error line from a full usage dump",
			in:   "Usage:\n  td create [title] [flags]\n\nFlags:\n  -h, --help\n\nError: title 'test' is too generic - describe what it does or fixes",
			want: "title 'test' is too generic - describe what it does or fixes",
		},
		{
			name: "falls back to the last non-empty line when there's no Error: prefix",
			in:   "something went wrong\n\n",
			want: "something went wrong",
		},
		{
			name: "single line passes through",
			in:   "title too short (3 chars, need 15)",
			want: "title too short (3 chars, need 15)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := tdErrorMessage(c.in); got != c.want {
				t.Errorf("tdErrorMessage()\n got: %q\nwant: %q", got, c.want)
			}
		})
	}
}

func TestOrderTasks(t *testing.T) {
	ts := func(s string) time.Time { v, _ := time.Parse(time.RFC3339, s); return v }
	got := orderTasks([]task.Task{
		{ID: "td-1", Status: "closed", UpdatedAt: ts("2026-05-27T10:00:00Z")},
		{ID: "td-2", Status: "open"},
		{ID: "td-3", Status: "in_progress", UpdatedAt: ts("2026-05-28T10:00:00Z")},
		{ID: "td-4", Status: "in_review", UpdatedAt: ts("2026-05-28T12:00:00Z")},
	})
	// open first, then active (most-recently-updated first), then closed.
	want := []string{"td-2", "td-4", "td-3", "td-1"}
	for i, w := range want {
		if got[i].ID != w {
			t.Errorf("position %d: got %s want %s", i, got[i].ID, w)
		}
	}
}
