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
		},
		Agents: []hub.AgentView{
			{Name: "brokkr", Role: "worker", Running: true, Phase: "working", Task: "td-feat1", PR: "pr-td-feat1"},
			{Name: "rune", Role: "reviewer", Running: true, Phase: "idle"},
			{Name: "dvalin", Role: "worker", Running: false, Phase: "idle"},
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
}
