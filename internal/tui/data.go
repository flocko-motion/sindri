package tui

import (
	"encoding/json"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/flo-at/sindri/internal/worker"

	tea "github.com/charmbracelet/bubbletea"
)

type taskItem struct {
	ID        string
	Title     string
	Status    string
	Type      string
	Priority  string
	Labels    []string
	CreatedAt time.Time
	UpdatedAt time.Time
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
		ID        string   `json:"id"`
		Title     string   `json:"title"`
		Status    string   `json:"status"`
		Type      string   `json:"type"`
		Priority  string   `json:"priority"`
		Labels    []string `json:"labels"`
		CreatedAt string   `json:"created_at"`
		UpdatedAt string   `json:"updated_at"`
	}
	if json.Unmarshal(out, &raw) != nil {
		return nil
	}
	items := make([]taskItem, len(raw))
	for i, r := range raw {
		created, _ := time.Parse(time.RFC3339Nano, r.CreatedAt)
		updated, _ := time.Parse(time.RFC3339Nano, r.UpdatedAt)
		items[i] = taskItem{
			ID:        r.ID,
			Title:     r.Title,
			Status:    r.Status,
			Type:      r.Type,
			Priority:  r.Priority,
			Labels:    r.Labels,
			CreatedAt: created,
			UpdatedAt: updated,
		}
	}
	// Three sections: open (by priority), in-progress (by updated_at desc), closed (by updated_at desc)
	var open, active, closed []taskItem
	for _, t := range items {
		switch t.Status {
		case "open":
			open = append(open, t)
		case "in_progress", "in_review":
			active = append(active, t)
		default:
			closed = append(closed, t)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		return active[i].UpdatedAt.After(active[j].UpdatedAt)
	})
	sort.Slice(closed, func(i, j int) bool {
		return closed[i].UpdatedAt.After(closed[j].UpdatedAt)
	})
	result := make([]taskItem, 0, len(items))
	result = append(result, open...)
	result = append(result, active...)
	result = append(result, closed...)
	return result
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

