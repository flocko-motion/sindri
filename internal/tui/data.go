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

type refreshMsg struct {
	issues  []issue.Issue
	workers []worker.Worker
	manual  bool
}

func refreshData(projectRoot string) tea.Cmd {
	return refreshDataOpt(projectRoot, false)
}

func refreshDataManual(projectRoot string) tea.Cmd {
	return refreshDataOpt(projectRoot, true)
}

func refreshDataOpt(projectRoot string, manual bool) tea.Cmd {
	return func() tea.Msg {
		issues, _ := board.List(projectRoot)
		workers := worker.List(projectRoot)
		return refreshMsg{issues: issues, workers: workers, manual: manual}
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
var (
	fetchTaskDetailFn   = realFetchTaskDetail
	fetchTaskCommentsFn = realFetchTaskComments
)

func fetchTaskDetail(projectRoot, taskID string) string {
	return fetchTaskDetailFn(projectRoot, taskID)
}

func fetchTaskComments(projectRoot, taskID string) string {
	return fetchTaskCommentsFn(projectRoot, taskID)
}

func realFetchTaskDetail(projectRoot, taskID string) string {
	out, err := td.Show(projectRoot, taskID)
	if err != nil {
		return "Error loading task: " + err.Error()
	}
	return out
}

func realFetchTaskComments(projectRoot, taskID string) string {
	out, err := td.Comments(projectRoot, taskID)
	if err != nil {
		return ""
	}
	return out
}

