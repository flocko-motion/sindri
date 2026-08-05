package cli

import (
	"errors"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// stubTaskInfoBackend answers only TaskInfo; embedding the (nil) backend interface means any
// other method panics if called — fine here, since agentTaskLabel calls nothing else.
type stubTaskInfoBackend struct {
	backend
	task api.Task
	err  error
}

func (s stubTaskInfoBackend) TaskInfo(string) (api.Task, error) { return s.task, s.err }

// TestAgentTaskLabel: `agent info` used to print only a task's bare id — no clue what an agent is
// actually working on. The label now includes the title when TaskInfo resolves it, and degrades
// to the bare id (never an error) when it can't — another repo, or a task since scrapped.
func TestAgentTaskLabel(t *testing.T) {
	cases := []struct {
		name string
		id   string
		b    backend
		want string
	}{
		{"no task", "", stubTaskInfoBackend{}, "-"},
		{"title found", "td-42", stubTaskInfoBackend{task: api.Task{Title: "fix the flaky test"}}, "td-42  fix the flaky test"},
		{"lookup fails", "td-42", stubTaskInfoBackend{err: errors.New("no such task")}, "td-42"},
		{"title empty", "td-42", stubTaskInfoBackend{task: api.Task{}}, "td-42"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := agentTaskLabel(c.b, c.id); got != c.want {
				t.Errorf("agentTaskLabel(%q) = %q, want %q", c.id, got, c.want)
			}
		})
	}
}
