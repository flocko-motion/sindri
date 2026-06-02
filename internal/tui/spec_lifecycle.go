// package: tui / spec_lifecycle
// type:    ui
// job:     Tea messages + commands that keep the td task and openspec change
//          lifecycles in sync — fired after a close/merge to auto-archive or
//          prompt, and from the x-on-spec-row abandon flow.
// limits:  no decision logic (-> action), no spec-CLI shelling
//          (-> adapter/spec). Just the UI glue.
package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flo-at/sindri/internal/action"
)

// specCheckMsg carries the post-close decision from action.MaybeArchiveLinkedSpec
// so the Update handler can either auto-archive, prompt, or do nothing.
type specCheckMsg struct {
	decision action.SpecAfterCloseDecision
	err      error
}

// specArchivedMsg and specAbandonedMsg are the result messages for the two
// spec-side mutations. They carry the spec name (for the user-facing
// notification) and either the closed-task list (abandon) or an error.
type specArchivedMsg struct {
	specName string
	err      error
}
type specAbandonedMsg struct {
	specName     string
	closedTasks  []string
	err          error
}

// checkLinkedSpecCmd asks action.MaybeArchiveLinkedSpec what to do next after
// a close/merge. Always safe to dispatch even when the closed task had no
// spec link — the decision will come back as None and the handler no-ops.
func checkLinkedSpecCmd(root, closedTaskID string) tea.Cmd {
	return func() tea.Msg {
		d, err := action.MaybeArchiveLinkedSpec(root, closedTaskID)
		return specCheckMsg{decision: d, err: err}
	}
}

func archiveSpecCmd(root, name string) tea.Cmd {
	return func() tea.Msg {
		err := action.ArchiveSpec(root, name)
		return specArchivedMsg{specName: name, err: err}
	}
}

func abandonSpecCmd(root, name string) tea.Cmd {
	return func() tea.Msg {
		closed, err := action.AbandonSpec(root, name)
		return specAbandonedMsg{specName: name, closedTasks: closed, err: err}
	}
}

// handleSpecLifecycle owns the Tea-message side of the td-task ↔ openspec
// lifecycle sync — extracted from the main Update so tui.go stays under the
// LOC limit and so this whole feature lives next to its commands.
//
// ok==true means the message was a spec-lifecycle message and the returned
// model/cmd should be used directly; ok==false means the caller should
// keep dispatching.
func (m Model) handleSpecLifecycle(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case mergeCompleteMsg:
		m.notify = notification{message: "PR merged: " + msg.prID, time: time.Now()}
		// The merge closed the linked td task; ask the lifecycle whether the
		// spec should also archive (or whether to prompt).
		return m, tea.Batch(
			flashTimer(),
			refreshAllCmd(m.projectRoot, false),
			checkLinkedSpecCmd(m.projectRoot, msg.taskID),
		), true
	case specCheckMsg:
		if msg.err != nil {
			m.notify = notification{message: "Spec check failed: " + msg.err.Error(), isError: true, time: time.Now()}
			return m, flashTimer(), true
		}
		switch msg.decision.Action {
		case action.SpecAfterCloseArchive:
			return m, archiveSpecCmd(m.projectRoot, msg.decision.SpecName), true
		case action.SpecAfterClosePrompt:
			m.confirmAction = "archive-spec:" + msg.decision.SpecName
			m.confirmLabel = fmt.Sprintf(
				"Last task for spec %s closed but checklist is %d/%d. Archive anyway? (y/n)",
				msg.decision.SpecName, msg.decision.ChecklistDone, msg.decision.ChecklistTotal)
			return m, nil, true
		}
		return m, nil, true
	case specArchivedMsg:
		if msg.err != nil {
			m.notify = notification{message: "Archive failed: " + msg.err.Error(), isError: true, time: time.Now()}
		} else {
			m.notify = notification{message: "Spec archived: " + msg.specName, time: time.Now()}
		}
		return m, tea.Batch(flashTimer(), refreshSpecsCmd(m.projectRoot)), true
	case specAbandonedMsg:
		if msg.err != nil {
			m.notify = notification{message: "Abandon failed: " + msg.err.Error(), isError: true, time: time.Now()}
			return m, flashTimer(), true
		}
		summary := "Spec abandoned: " + msg.specName
		if n := len(msg.closedTasks); n > 0 {
			summary = fmt.Sprintf("Spec abandoned: %s (closed %d linked task(s))", msg.specName, n)
		}
		m.notify = notification{message: summary, time: time.Now()}
		return m, tea.Batch(flashTimer(), refreshAllCmd(m.projectRoot, false)), true
	}
	return m, nil, false
}

