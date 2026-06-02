package action

import (
	"testing"

	"github.com/flo-at/sindri/internal/adapter/spec"
	"github.com/flo-at/sindri/internal/issue"
)

func TestDecideSpecAfterClose(t *testing.T) {
	mkTask := func(id, status string, labels ...string) issue.Task {
		return issue.Task{ID: id, Status: status, Labels: labels}
	}
	mkSpec := func(name string, done, total int) *spec.Change {
		return &spec.Change{Name: name, CompletedTasks: done, TotalTasks: total}
	}

	cases := []struct {
		name     string
		closedID string
		tasks    []issue.Task
		active   *spec.Change
		want     SpecAfterCloseDecision
	}{
		{
			name:     "task not in board → no action",
			closedID: "td-missing",
			tasks:    []issue.Task{mkTask("td-a", "open")},
			active:   mkSpec("foo", 0, 3),
			want:     SpecAfterCloseDecision{},
		},
		{
			name:     "task has no spec label → no action",
			closedID: "td-a",
			tasks:    []issue.Task{mkTask("td-a", "closed")},
			active:   nil,
			want:     SpecAfterCloseDecision{},
		},
		{
			name:     "spec already archived (active == nil) → no action",
			closedID: "td-a",
			tasks:    []issue.Task{mkTask("td-a", "closed", "spec:foo")},
			active:   nil,
			want:     SpecAfterCloseDecision{},
		},
		{
			name:     "other open linked task remains → no action",
			closedID: "td-a",
			tasks: []issue.Task{
				mkTask("td-a", "closed", "spec:foo"),
				mkTask("td-b", "in_progress", "spec:foo"),
			},
			active: mkSpec("foo", 1, 3),
			want:   SpecAfterCloseDecision{},
		},
		{
			name:     "last linked task closed + checklist complete → archive",
			closedID: "td-a",
			tasks:    []issue.Task{mkTask("td-a", "closed", "spec:foo")},
			active:   mkSpec("foo", 3, 3),
			want:     SpecAfterCloseDecision{Action: SpecAfterCloseArchive, SpecName: "foo", ChecklistDone: 3, ChecklistTotal: 3},
		},
		{
			name:     "last linked task closed + no checklist (0/0) → archive",
			closedID: "td-a",
			tasks:    []issue.Task{mkTask("td-a", "closed", "spec:foo")},
			active:   mkSpec("foo", 0, 0),
			want:     SpecAfterCloseDecision{Action: SpecAfterCloseArchive, SpecName: "foo"},
		},
		{
			name:     "last linked task closed + checklist incomplete → prompt",
			closedID: "td-a",
			tasks:    []issue.Task{mkTask("td-a", "closed", "spec:foo")},
			active:   mkSpec("foo", 2, 5),
			want:     SpecAfterCloseDecision{Action: SpecAfterClosePrompt, SpecName: "foo", ChecklistDone: 2, ChecklistTotal: 5},
		},
		{
			name:     "another task with different spec doesn't block archive",
			closedID: "td-a",
			tasks: []issue.Task{
				mkTask("td-a", "closed", "spec:foo"),
				mkTask("td-b", "open", "spec:bar"),
			},
			active: mkSpec("foo", 1, 1),
			want:   SpecAfterCloseDecision{Action: SpecAfterCloseArchive, SpecName: "foo", ChecklistDone: 1, ChecklistTotal: 1},
		},
		{
			name:     "another closed linked task doesn't block archive",
			closedID: "td-a",
			tasks: []issue.Task{
				mkTask("td-a", "closed", "spec:foo"),
				mkTask("td-old", "closed", "spec:foo"),
			},
			active: mkSpec("foo", 2, 2),
			want:   SpecAfterCloseDecision{Action: SpecAfterCloseArchive, SpecName: "foo", ChecklistDone: 2, ChecklistTotal: 2},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decideSpecAfterClose(tc.closedID, tc.tasks, tc.active)
			if got != tc.want {
				t.Errorf("decideSpecAfterClose() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
