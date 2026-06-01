// package: tui / replay_fixtures
// type:    ui
// job:     deterministic Go constructors for Fixture values the replay engine
//          uses in golden tests; static data, no I/O.
// limits:  test data only; production paths get their issues/workers from
//          board.List + worker.List.
package tui

import (
	"time"

	"github.com/flo-at/sindri/internal/issue"
	"github.com/flo-at/sindri/internal/worker"
)

// SimpleFixture returns a small, deterministic board covering the states the
// goldens care about: one spec-only Issue, one open task, one in-progress task
// with a worker and a PR (one met review gate), and one closed task. Workers
// include a running dwarf, an idle dwarf, and the reviewer.
func SimpleFixture() Fixture {
	t0 := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	openTask := &issue.Task{
		ID:        "td-aaaaaa",
		Title:     "fix login redirect",
		Status:    "open",
		Type:      "bug",
		Priority:  "P1",
		Labels:    []string{"require-review-code"},
		CreatedAt: t0.Add(-3 * time.Hour),
		UpdatedAt: t0.Add(-2 * time.Hour),
	}
	inProgressTask := &issue.Task{
		ID:        "td-bbbbbb",
		Title:     "add CSV export",
		Status:    "in_progress",
		Type:      "feature",
		Priority:  "P2",
		Labels:    []string{"spec:csv-export", "require-review-code", "approved-review-code"},
		CreatedAt: t0.Add(-24 * time.Hour),
		UpdatedAt: t0,
	}
	closedTask := &issue.Task{
		ID:        "td-cccccc",
		Title:     "rename logger",
		Status:    "closed",
		Type:      "chore",
		Priority:  "P3",
		CreatedAt: t0.Add(-72 * time.Hour),
		UpdatedAt: t0.Add(-48 * time.Hour),
	}

	authSpec := &issue.Spec{Name: "auth-refactor", CompletedTasks: 0, TotalTasks: 3}
	csvSpec := &issue.Spec{Name: "csv-export", CompletedTasks: 1, TotalTasks: 2}

	openPR := issue.PR{
		ID:     "pr-td-bbbbbb",
		Status: "open",
		Branch: "td-bbbbbb",
		Base:   "master",
		Title:  "feat(td-bbbbbb): add CSV export",
	}

	issues := []issue.Issue{
		{Spec: authSpec},                                                              // spec-only
		{Task: openTask},                                                              // open
		{Task: inProgressTask, Worker: "brokkr", PRs: []issue.PR{openPR}, Spec: csvSpec}, // in progress
		{Task: closedTask},                                                            // closed (hidden by default filter)
	}

	workers := []worker.Worker{
		{
			Name: "brokkr", Role: "worker", Status: "running",
			Task: "td-bbbbbb add CSV export", PR: "pr-td-bbbbbb [open]",
			Path: "/proj/.worktrees/brokkr", Branch: "td-bbbbbb",
			Container: "sindri-brokkr",
		},
		{
			Name: "dvalin", Role: "worker", Status: "-",
			Path: "/proj/.worktrees/dvalin",
		},
		{
			Name: "reviewer", Role: "reviewer", Status: "-",
			Path: "/proj/.worktrees/reviewer",
		},
	}

	return Fixture{
		Issues: issues, Workers: workers, Width: 100, Height: 30,
		Descriptions: map[string]string{
			"td-bbbbbb": "Export the issue list as CSV from the toolbar action.\n\n" +
				"Acceptance: header row, one row per task, UTF-8.",
		},
		Comments: map[string]string{
			"td-bbbbbb": "brokkr: PR is ready for review, gates met.",
		},
	}
}
