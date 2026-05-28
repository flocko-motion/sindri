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

func fetchTaskDetail(projectRoot, taskID string) string {
	out, err := td.Show(projectRoot, taskID)
	if err != nil {
		return "Error loading task: " + err.Error()
	}
	return out
}

func fetchTaskComments(projectRoot, taskID string) string {
	out, err := td.Comments(projectRoot, taskID)
	if err != nil {
		return ""
	}
	return out
}

