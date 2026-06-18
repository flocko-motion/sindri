package tui

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/muesli/termenv"
)

// mockBoard is a representative board: a task tree (epic→feature→tasks) with a
// standalone bug, agents (running/stopped) + an orphan, and an open PR.
func mockBoard() hub.BoardState {
	return hub.BoardState{
		Tasks: []store.Task{
			{ID: "td-ep01", Title: "Authentication epic", Status: "open", Priority: "P1", Type: "epic"},
			{ID: "td-feat1", Title: "Login feature", Status: "in_progress", Priority: "P1", Type: "feature", ParentID: "td-ep01"},
			{ID: "td-t1", Title: "Wire the login form", Status: "open", Priority: "P2", Type: "task", ParentID: "td-feat1"},
			{ID: "td-t2", Title: "Session handling", Status: "closed", Priority: "P3", Type: "task", ParentID: "td-feat1"},
			{ID: "td-feat2", Title: "Signup feature", Status: "open", Priority: "P2", Type: "feature", ParentID: "td-ep01"},
			{ID: "td-bug9", Title: "Crash on empty input", Status: "open", Priority: "P0", Type: "bug"},
			{ID: "os-a1b2c3", Title: "hub-architecture (15/15)", Status: "open", Type: "spec"},
			{ID: "os-d4e5f6", Title: "tui-dashboard (22/23)", Status: "open", Type: "spec"},
		},
		Agents: []hub.AgentView{
			{Name: "brokkr", Role: "worker", Status: "working", Task: "td-feat1", PR: "pr-td-feat1", Workspace: ".worktrees/brokkr"},
			{Name: "rune", Role: "reviewer", Status: "idle", Workspace: ".worktrees/rune"},
			{Name: "dvalin", Role: "worker", Status: "down", Workspace: ".worktrees/dvalin"},
		},
		PRs: []store.PR{
			{ID: "pr-td-feat1", Task: "td-feat1", Agent: "brokkr", Branch: "td-feat1", Base: "master", Status: "open"},
		},
		Orphans: []string{"sindri-ghost"},
	}
}

// TestScreenshot renders each tab so the layout can be eyeballed:
//
//	go test ./internal/tui/ -run Screenshot -v
func TestScreenshot(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii) // plain text, no escape codes
	b := mockBoard()
	scenes := []struct {
		name string
		keys []string
	}{
		{"Tasks tab (default: open filter)", nil},
		{"Tasks tab — cursor on the feature", []string{"j", "j"}},
		{"Tasks tab — filter=all (shows closed)", []string{"f", "f"}},
		{"Agents tab", []string{"2"}},
		{"PRs tab", []string{"3"}},
	}
	for _, sc := range scenes {
		fmt.Printf("\n========== %s ==========\n%s\n", sc.name, Screenshot(b, 96, 20, sc.keys...))
	}
	// Detail modal (ENTER) and a narrow terminal (detail pane hidden).
	fmt.Printf("\n========== detail modal (ENTER on a task) ==========\n%s\n", Screenshot(b, 96, 20, "j", "j", "enter"))
	fmt.Printf("\n========== narrow terminal (70 wide — no detail pane) ==========\n%s\n", Screenshot(b, 70, 16))
	fmt.Printf("\n========== priority choice modal (p on a task) ==========\n%s\n", Screenshot(b, 70, 16, "p"))
	fmt.Printf("\n========== new-task form (n) ==========\n%s\n", Screenshot(b, 80, 22, "n"))
	// Focus the description textarea (tab×5) and type a multi-line body.
	fmt.Printf("\n========== new-task form — description textarea focused ==========\n%s\n",
		Screenshot(b, 80, 22, "n", "tab", "tab", "tab", "tab", "tab", "f", "i", "x", " ", "i", "t", "enter", "n", "o", "w"))
	// Parent validation: type a bogus parent then try to save (ctrl+s blocks).
	fmt.Printf("\n========== new-task form — parent validation error ==========\n%s\n",
		Screenshot(b, 80, 22, "n", "tab", "tab", "tab", "x", "y", "z", "ctrl+s"))
	fmt.Printf("\n========== edit-task form (e on the bug) ==========\n%s\n", Screenshot(b, 80, 22, "j", "j", "j", "j", "j", "e"))
	fmt.Printf("\n========== Agents tab — wide 3-region layout (list+tmux | detail) ==========\n%s\n", Screenshot(b, 150, 24, "2"))
	fmt.Printf("\n========== agent role choice (2, e) ==========\n%s\n", Screenshot(b, 70, 16, "2", "e"))
	fmt.Printf("\n========== agent delete confirm (2, d) ==========\n%s\n", Screenshot(b, 70, 16, "2", "d"))
	fmt.Printf("\n========== error modal (attach a down agent) ==========\n%s\n", Screenshot(b, 80, 16, "2", "j", "j", "a"))
}
