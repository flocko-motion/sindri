// package: tui / data
// type:    ui
// job:     TUI data plumbing — drives the Bubble Tea refresh from board.List
//          and worker.List, and reads task detail/comments via adapter/td.
// limits:  no domain logic (-> issue/board), no styling (-> render).
package tui

import (
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/board"
	"github.com/flo-at/sindri/internal/issue"
	"github.com/flo-at/sindri/internal/worker"

	tea "github.com/charmbracelet/bubbletea"
)

// Per-source refresh messages. Each loader emits its own message so the
// Update handler can patch m.boardData independently and re-Assemble; the TUI
// paints as soon as the first message lands.
type tasksRefreshedMsg struct {
	tasks  []issue.Task
	manual bool
}
type specsRefreshedMsg struct{ specs []issue.Spec }
type workersRefreshedMsg struct{ workers []worker.Worker }
type prsRefreshedMsg struct{ prs map[string][]issue.PR }

// refreshAllCmd dispatches all four loaders in parallel. Used by Init, the
// periodic tick, and manual refresh.
func refreshAllCmd(projectRoot string, manual bool) tea.Cmd {
	return tea.Batch(
		refreshTasksCmd(projectRoot, manual),
		refreshSpecsCmd(projectRoot),
		refreshWorkersCmd(projectRoot),
		refreshPRsCmd(projectRoot),
	)
}

// refreshTasksCmd fetches only td tasks. Used as the lightweight post-mutation
// path so podman and openspec are not contacted for things that didn't change.
func refreshTasksCmd(projectRoot string, manual bool) tea.Cmd {
	return func() tea.Msg {
		tasks, _ := board.LoadTasks(projectRoot)
		return tasksRefreshedMsg{tasks: tasks, manual: manual}
	}
}

func refreshSpecsCmd(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		return specsRefreshedMsg{specs: board.LoadSpecs(projectRoot)}
	}
}

func refreshWorkersCmd(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		return workersRefreshedMsg{workers: board.LoadWorkers(projectRoot)}
	}
}

func refreshPRsCmd(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		return prsRefreshedMsg{prs: board.LoadPRs(projectRoot)}
	}
}

// cacheWarmedMsg is dispatched once the background warmCacheCmd finishes
// populating the parent-id cache, so the next refresh shows hierarchy.
type cacheWarmedMsg struct{}

// warmCacheCmd runs WarmParentCache off the foreground refresh path. It costs
// several seconds today (each `td show` is ~250ms and td's DB locks defeat
// useful parallelism), but it only runs once per session — the in-process
// cache makes every subsequent List instant. When done it fires a
// cacheWarmedMsg so the model can trigger one more refresh and finally render
// the hierarchy.
func warmCacheCmd(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		board.WarmParentCache(projectRoot)
		return cacheWarmedMsg{}
	}
}

// fetchTaskDetailFn and fetchTaskCommentsFn are package-level seams so the
// replay engine can swap real td shell-outs for fixture lookups. Production
// callers go through fetchTaskDetail/Comments below, which delegate here.
//
// fetchTaskDetail returns the task's *description body only* — the metadata
// (ID/status/type/etc.) lives in the left pane of the detail view, and
// echoing it again on the right used to produce visible duplication.
// fetchTaskAcceptance returns acceptance criteria similarly.
var (
	fetchTaskDetailFn     = realFetchTaskDetail
	fetchTaskAcceptanceFn = realFetchTaskAcceptance
	fetchTaskCommentsFn   = realFetchTaskComments
)

func fetchTaskDetail(projectRoot, taskID string) string {
	return fetchTaskDetailFn(projectRoot, taskID)
}

func fetchTaskAcceptance(projectRoot, taskID string) string {
	return fetchTaskAcceptanceFn(projectRoot, taskID)
}

func fetchTaskComments(projectRoot, taskID string) string {
	return fetchTaskCommentsFn(projectRoot, taskID)
}

func realFetchTaskDetail(projectRoot, taskID string) string {
	desc, _, err := td.Detail(projectRoot, taskID)
	if err != nil {
		return "Error loading task: " + err.Error()
	}
	return desc
}

func realFetchTaskAcceptance(projectRoot, taskID string) string {
	_, acc, err := td.Detail(projectRoot, taskID)
	if err != nil {
		return ""
	}
	return acc
}

func realFetchTaskComments(projectRoot, taskID string) string {
	out, err := td.Comments(projectRoot, taskID)
	if err != nil {
		return ""
	}
	return out
}

