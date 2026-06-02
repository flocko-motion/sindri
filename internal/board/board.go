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

	"github.com/flo-at/sindri/internal/adapter/spec"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/flo-at/sindri/internal/issue"
	"github.com/flo-at/sindri/internal/worker"
)

// List returns the unified, ordered work items for a project: spec-only items
// first, then tasks (open → active → closed), each with its spec, worker, and
// PRs attached. This is the one refresh both interfaces use so they always
// show the same data.
func List(projectRoot string) ([]issue.Issue, error) {
	tasks, err := td.Tasks(projectRoot)
	if err != nil {
		return nil, err
	}
	// td list strips parent_id; per-task show fills it in (~5ms each).
	td.Enrich(projectRoot, tasks)

	specs := specsFor(projectRoot)
	workerByTask := workerAssignments(projectRoot)
	prsByTask := prsByTask(projectRoot)

	return issue.Assemble(tasks, specs, workerByTask, prsByTask), nil
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

func workerAssignments(projectRoot string) map[string]string {
	m := map[string]string{}
	for _, wk := range worker.List(projectRoot) {
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
