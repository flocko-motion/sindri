package tui

import (
	"os/exec"
	"strings"

	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/flo-at/sindri/internal/issue"
	"github.com/flo-at/sindri/internal/worker"

	tea "github.com/charmbracelet/bubbletea"
)

// taskItem is the headless task model; the TUI renders it but owns no logic.
type taskItem = issue.Task

type prItem struct {
	ID     string
	Branch string
	Base   string
	Status string
	Title  string
}

type refreshMsg struct {
	workers []worker.Worker
	tasks   []taskItem
	prs     []prItem
	err     error
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
		workers := worker.List(projectRoot)
		tasks := fetchTasks(projectRoot)
		prs := fetchPRs(projectRoot)
		return refreshMsg{workers: workers, tasks: tasks, prs: prs, manual: manual}
	}
}

func fetchTasks(projectRoot string) []taskItem {
	tasks, err := issue.LoadTasks(projectRoot)
	if err != nil {
		return nil
	}
	return tasks
}

func fetchPRs(projectRoot string) []prItem {
	prs, err := store.ListFor(projectRoot)
	if err != nil {
		return nil
	}
	items := make([]prItem, 0, len(prs))
	for _, pr := range prs {
		items = append(items, prItem{
			ID:     pr.ID,
			Branch: pr.Branch,
			Base:   pr.Base,
			Status: pr.Status,
			Title:  pr.Title,
		})
	}
	return items
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

