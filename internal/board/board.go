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

// parentCache holds a per-process taskID → parentID map. td list strips
// parent_id and td show is slow (~250ms each, DB locks prevent useful
// parallelism), so we read whatever the cache has and let WarmParentCache fill
// it in the background. Subsequent refreshes are then instant.
var parentCache sync.Map // map[string]string

// WarmParentCache refreshes the parent_id cache for a project by running
// td.Enrich and storing each task's ParentID. Callers run this in a goroutine
// when they can tolerate the latency (~7s parallel for 50 tasks); board.List
// itself never calls it.
func WarmParentCache(projectRoot string) {
	tasks, err := td.Tasks(projectRoot)
	if err != nil {
		return
	}
	td.Enrich(projectRoot, tasks)
	for _, t := range tasks {
		parentCache.Store(t.ID, t.ParentID)
	}
}

func cachedParent(id string) string {
	if v, ok := parentCache.Load(id); ok {
		return v.(string)
	}
	return ""
}

// SetCachedParent updates the in-process parent-id cache after a mutation
// (e.g. an interactive move), so the next refresh sees the new hierarchy
// without re-running WarmParentCache.
func SetCachedParent(id, parent string) {
	parentCache.Store(id, parent)
}

// List returns the unified, ordered work items for a project: spec-only items
// first, then tasks (open → active → closed), each with its spec, worker, and
// PRs attached. This is the one refresh both interfaces use so they always
// show the same data.
func List(projectRoot string) ([]issue.Issue, error) {
	// The four data sources are independent; serializing them adds the slow
	// ones (openspec ~1.2s, podman ~1.5s) to td (~0.3s) for a ~3s stall on
	// first paint. Fan them out and wait on the slowest.
	var (
		tasks      []issue.Task
		taskErr    error
		specs      []issue.Spec
		workerByID map[string]string
		prsByID    map[string][]issue.PR
		wg         sync.WaitGroup
	)
	wg.Add(4)
	go func() {
		defer wg.Done()
		ts, err := td.Tasks(projectRoot)
		if err != nil {
			taskErr = err
			return
		}
		// td list strips parent_id; populate from the in-process cache so the
		// hierarchy renders without the slow td.show round-trip. A separate
		// WarmParentCache goroutine keeps the cache fresh in the background.
		for i := range ts {
			if ts[i].ParentID == "" {
				ts[i].ParentID = cachedParent(ts[i].ID)
			}
		}
		tasks = ts
	}()
	go func() {
		defer wg.Done()
		specs = specsFor(projectRoot)
	}()
	go func() {
		defer wg.Done()
		workerByID = workerAssignments(projectRoot)
	}()
	go func() {
		defer wg.Done()
		prsByID = prsByTask(projectRoot)
	}()
	wg.Wait()
	if taskErr != nil {
		return nil, taskErr
	}
	return issue.Assemble(tasks, specs, workerByID, prsByID), nil
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
