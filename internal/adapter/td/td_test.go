package td

import (
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/issue"
)

func TestOrderTasks(t *testing.T) {
	ts := func(s string) time.Time { v, _ := time.Parse(time.RFC3339, s); return v }
	got := orderTasks([]issue.Task{
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
