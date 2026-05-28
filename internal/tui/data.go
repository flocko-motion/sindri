package tui

import (
	"os/exec"
	"strings"

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
	out, err := exec.Command("td", "-w", projectRoot, "show", taskID).Output()
	if err != nil {
		return "Error loading task: " + err.Error()
	}
	return strings.TrimSpace(string(out))
}

func fetchTaskComments(projectRoot, taskID string) string {
	out, err := exec.Command("td", "-w", projectRoot, "comments", taskID).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

