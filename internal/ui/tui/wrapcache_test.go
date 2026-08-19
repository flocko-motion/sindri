// package: tui / wrapcache_test
// type:    ui (tests for detailWrap's cache, sd-ad43a1)
// job:     pins that a cursor move within one task's detail costs no re-wrap, and that a
// genuine new selection still gets one — a call count, not a timing, since timing is flaky.
// limits:  the Tasks tab only; reclamp's other cached tabs (3, 5, 6) share the same helper.
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flo-at/sindri/internal/api"
)

// TestCursorMoveWithinOneTaskDoesNotRewrap: reclamp used to re-wrap the whole detail body — ANSI-
// aware, proportional to its length — on every keystroke, even one that moved the cursor within the
// same task's actionable items and changed neither content nor geometry.
func TestCursorMoveWithinOneTaskDoesNotRewrap(t *testing.T) {
	var comments []api.Comment
	for i := 0; i < 50; i++ {
		comments = append(comments, api.Comment{
			Author: "eitri", Source: "sindri", CreatedAt: "2026-01-01T00:00:00Z",
			Body: "a long enough finding that word-wrapping it is not free",
		})
	}
	task := api.Task{
		ID: "td-t1", Title: "a task with a long thread", Status: "open", Priority: "P1", Type: "task",
		URL: "https://example.com/1", ParentID: "td-parent", Comments: comments,
	}
	var tm tea.Model = newModel(nil, nil, "") // already reclamps once, against the empty "(no task)" state
	m := tm.(model)
	beforeContent := m.wrapCalls
	m.w, m.h = 96, 24
	m.state = api.BoardState{Tasks: []api.Task{task}}
	m.taskDetail = task
	m.detailKey = "0:td-t1" // pretend the lazy fetch already landed
	m.reclamp()
	if m.wrapCalls != beforeContent+1 {
		t.Fatalf("loading the task's content should wrap once more, went from %d to %d", beforeContent, m.wrapCalls)
	}
	tm = m

	tm, _ = tm.Update(keyMsg("ctrl+l")) // focus the detail column: parent/url are actionable
	m = tm.(model)
	if !m.rightFocus {
		t.Fatal("ctrl+l should focus the detail column")
	}
	afterFocus := m.wrapCalls

	for i := 0; i < 6; i++ { // moves rightCursor among the SAME task's actionable items
		tm, _ = tm.Update(keyMsg("j"))
	}
	m = tm.(model)
	if m.wrapCalls != afterFocus {
		t.Errorf("moving within one task's detail re-wrapped: wrapCalls went from %d to %d", afterFocus, m.wrapCalls)
	}

	// Sanity: a genuine new selection still re-wraps — the cache does not get stuck.
	m.state.Tasks = append(m.state.Tasks, api.Task{ID: "td-t2", Title: "a second, different task", Status: "open"})
	m.reclamp()
	tm = m
	tm, _ = tm.Update(keyMsg("ctrl+h")) // back to the list
	tm, _ = tm.Update(keyMsg("j"))      // select the other task
	m = tm.(model)
	if m.wrapCalls == afterFocus {
		t.Error("selecting a different task should re-wrap, not reuse the old task's cache")
	}
}
