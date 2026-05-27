package tui

import (
	"encoding/json"
	"os/exec"
	"strings"

	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/flo-at/sindri/internal/worker"

	tea "github.com/charmbracelet/bubbletea"
)

type taskItem struct {
	ID       string
	Title    string
	Status   string
	Type     string
	Priority string
	Labels   []string
}

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
}

func refreshData(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		workers := worker.List(projectRoot)
		tasks := fetchTasks(projectRoot)
		prs := fetchPRs(projectRoot)
		return refreshMsg{workers: workers, tasks: tasks, prs: prs}
	}
}

func fetchTasks(projectRoot string) []taskItem {
	out, err := exec.Command("td", "-w", projectRoot, "list", "--json", "--limit", "100", "--all").Output()
	if err != nil {
		return nil
	}
	var raw []struct {
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		Status   string   `json:"status"`
		Type     string   `json:"type"`
		Priority string   `json:"priority"`
		Labels   []string `json:"labels"`
	}
	if json.Unmarshal(out, &raw) != nil {
		return nil
	}
	items := make([]taskItem, len(raw))
	for i, r := range raw {
		items[i] = taskItem{
			ID:       r.ID,
			Title:    r.Title,
			Status:   r.Status,
			Type:     r.Type,
			Priority: r.Priority,
			Labels:   r.Labels,
		}
	}
	return items
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
