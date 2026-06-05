// package: board
// type:    assembly
// job:     the single refresh path — fetches td tasks, openspec changes, worker
//          assignments, and PRs, then derives one []issue.Issue via the pure
//          issue.Assemble. Both UIs call board.List.
// limits:  no rendering (-> render) and no interface code (-> cmd/sindri,
//          internal/tui); it only gathers and assembles.
package board

import (
	"strings"
	"sync"

	"github.com/flo-at/sindri/internal/adapter/spec"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/flo-at/sindri/internal/issue"
	"github.com/flo-at/sindri/internal/worker"
)

// LoadTasks fetches td tasks for a project. parent_id (hierarchy) comes
// straight from `td list --json`, so no enrichment round-trip is needed. Used
// as one of the four independent loaders the TUI dispatches per-source.
func LoadTasks(projectRoot string, f issue.Filter) ([]issue.Task, error) {
	return td.Tasks(projectRoot, f)
}

// LoadSpecs fetches openspec changes for a project. Returns nil when the
// project doesn't use openspec.
func LoadSpecs(projectRoot string) []issue.Spec {
	return specsFor(projectRoot)
}

// LoadWorkers fetches the full worker list for a project (podman ps).
func LoadWorkers(projectRoot string) []worker.Worker {
	return worker.List(projectRoot)
}

// WorkerByID converts a worker list to the `taskID → workerName` map that
// issue.Assemble consumes. Pure helper, no I/O.
func WorkerByID(workers []worker.Worker) map[string]string {
	m := map[string]string{}
	for _, wk := range workers {
		if wk.Task == "" {
			continue
		}
		// wk.Task is "td-xxxxxx Some title"; the ID is the first field.
		if parts := strings.Fields(wk.Task); len(parts) > 0 {
			m[parts[0]] = wk.Name
		}
	}
	return m
}

// LoadPRs fetches the PRs grouped by task ID for a project.
func LoadPRs(projectRoot string) map[string][]issue.PR {
	return prsByTask(projectRoot)
}

// List runs the four loaders in parallel and assembles their result. Used by
// the CLI's one-shot list and view commands; the TUI fans the loaders out
// individually so it can paint as soon as the first one returns. The filter is
// pushed down to the task query so closed tasks don't crowd out open ones.
func List(projectRoot string, f issue.Filter) ([]issue.Issue, error) {
	var (
		tasks      []issue.Task
		taskErr    error
		specs      []issue.Spec
		workers    []worker.Worker
		prsByID    map[string][]issue.PR
		wg         sync.WaitGroup
	)
	wg.Add(4)
	go func() { defer wg.Done(); tasks, taskErr = LoadTasks(projectRoot, f) }()
	go func() { defer wg.Done(); specs = LoadSpecs(projectRoot) }()
	go func() { defer wg.Done(); workers = LoadWorkers(projectRoot) }()
	go func() { defer wg.Done(); prsByID = LoadPRs(projectRoot) }()
	wg.Wait()
	if taskErr != nil {
		return nil, taskErr
	}
	return issue.Assemble(tasks, specs, WorkerByID(workers), prsByID), nil
}

func specsFor(projectRoot string) []issue.Spec {
	changes := spec.Changes(projectRoot)
	specs := make([]issue.Spec, 0, len(changes))
	for _, ch := range changes {
		specs = append(specs, issue.Spec{
			Name:           ch.Name,
			CompletedTasks: ch.CompletedTasks,
			TotalTasks:     ch.TotalTasks,
		})
	}
	return specs
}

func prsByTask(projectRoot string) map[string][]issue.PR {
	prs, err := store.ListFor(projectRoot)
	if err != nil {
		return nil
	}
	m := map[string][]issue.PR{}
	for _, pr := range prs {
		id := issue.TaskIDFromTitle(pr.Title)
		if id == "" {
			continue
		}
		m[id] = append(m[id], issue.PR{
			ID:     pr.ID,
			Status: pr.Status,
			Branch: pr.Branch,
			Base:   pr.Base,
			Title:  pr.Title,
		})
	}
	return m
}
