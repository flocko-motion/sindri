package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestTasksJSON: `task list --json` renders the task rows as a JSON array using
// their json tags, and yields an empty array (not null) when there are no tasks.
func TestTasksJSON(t *testing.T) {
	// Empty/nil input → "[]", never "null", so consumers can always parse an array.
	for _, in := range [][]store.Task{nil, {}} {
		got, err := tasksJSON(in)
		if err != nil {
			t.Fatalf("tasksJSON(%v): %v", in, err)
		}
		if got != "[]" {
			t.Errorf("empty tasks = %q, want %q", got, "[]")
		}
	}

	tasks := []store.Task{
		{ID: "td-1", Title: "first", Status: "open", Priority: "P1", Type: "feature", Labels: "a,b"},
		{ID: "td-2", Title: "second", Status: "closed"},
	}
	got, err := tasksJSON(tasks)
	if err != nil {
		t.Fatalf("tasksJSON: %v", err)
	}

	// It round-trips back to the same rows (json tags carry the field names).
	var back []store.Task
	if err := json.Unmarshal([]byte(got), &back); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, got)
	}
	if len(back) != 2 || back[0].ID != "td-1" || back[1].Status != "closed" {
		t.Fatalf("round-trip mismatch: %+v", back)
	}
	// Uses the struct's snake_case json tags, and indents for readability.
	if !strings.Contains(got, `"id": "td-1"`) || !strings.Contains(got, `"parent_id"`) {
		t.Errorf("expected json-tagged, indented output, got:\n%s", got)
	}
}
