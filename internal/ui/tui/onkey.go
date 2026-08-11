// package: tui / onkey
// type:    ui (key dispatch)
// job:     apply one key press to the model — the single switch every binding in keys.go routes
// to, shared by the live loop and the headless Screenshot harness.
// limits:  mutates the model and returns a cmd; the loop that calls it lives in tui.go.
package tui

import (
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/flo-at/sindri/internal/api"
)

// onKey applies a key by its string form, shared by the live loop and the headless Screenshot
// harness. Mutates the model; returns an optional cmd.
func (m *model) onKey(k string) tea.Cmd {
	oldTab := m.tab
	m.flash = "" // any keypress clears the previous transient status
	switch k {
	case "y": // yank: the focused right-column value, else the selected id
		if m.rightFocus {
			if it, ok := m.focusedItem(); ok {
				_ = clipboard.WriteAll(it.value)
				m.flash = "copied: " + it.value
			}
			return nil
		}
		// From the PRs list, the id alone rarely says enough to paste anywhere — the block naming
		// the PR, its task and its worktree does. The detail pane keeps yanking one field.
		if m.tab == 2 {
			if block := m.prYankBlock(); len(block) > 0 {
				_ = clipboard.WriteAll(strings.Join(block, "\n"))
				m.flash = "copied PR details"
				return nil
			}
		}
		if id := m.selID(); id != "" {
			_ = clipboard.WriteAll(id)
			m.flash = "copied id: " + id
		}
		return nil
	case "Y": // yank the selection's full details
		if m.selID() != "" {
			_ = clipboard.WriteAll(strings.Join(m.detailLines(), "\n"))
			m.flash = "copied full details"
		}
		return nil
	}
	switch k {
	case keyQuit, "ctrl+c":
		m.quit = true
		return nil
	case "tab", "]": // switch tabs forward (] mirrors tab)
		m.tab = (m.tab + 1) % len(tuiSections)
	case "shift+tab", "[": // switch tabs back ([ mirrors shift+tab)
		m.tab = (m.tab - 1 + len(tuiSections)) % len(tuiSections)
	case "ctrl+l": // the only way to switch panes (with ctrl+h): focus the detail
		if m.showDetail() && len(m.actionableItems()) > 0 {
			m.rightFocus = true
			m.rightCursor = clampInt(m.rightCursor, 0, max(0, len(m.actionableItems())-1))
		}
	case "ctrl+h": // focus back to the list
		m.rightFocus = false
	case "j", "down":
		if m.rightFocus {
			m.rightCursor = clampInt(m.rightCursor+1, 0, max(0, len(m.actionableItems())-1))
		} else {
			m.cursor[m.tab]++
		}
	case "k", "up":
		if m.rightFocus {
			m.rightCursor = clampInt(m.rightCursor-1, 0, max(0, len(m.actionableItems())-1))
		} else {
			m.cursor[m.tab]--
		}
	case "J": // scroll the detail pane down (yazi-style secondary-pane scroll)
		vp := m.scrollTarget()
		for i := 0; i < detailScrollStep; i++ {
			vp.ScrollDown()
		}
		return nil
	case "K": // scroll the detail pane up
		vp := m.scrollTarget()
		for i := 0; i < detailScrollStep; i++ {
			vp.ScrollUp()
		}
		return nil
	case "g": // goto the focused cross-reference's home, else jump the list to top
		if m.rightFocus {
			// gotoItem may switch tabs; fall through to the tail reclamp + syncDetail
			// so the destination tab's viewports are sized for it (not the old tab).
			if it, ok := m.focusedItem(); ok && it.kind != "path" {
				m.gotoItem(it.kind, it.value)
			}
		} else {
			m.cursor[m.tab] = 0
		}
	case "G":
		m.cursor[m.tab] = 1 << 30
	case "ctrl+d":
		if m.tab == 2 { // PRs: fast-scroll the diff/lint main pane (J/K do fine-grained)
			for i := 0; i < max(1, m.detail.Height/2); i++ {
				m.detail.ScrollDown()
			}
			return nil
		}
		m.cursor[m.tab] += m.bodyHeight() / 2
	case "ctrl+u":
		if m.tab == 2 {
			for i := 0; i < max(1, m.detail.Height/2); i++ {
				m.detail.ScrollUp()
			}
			return nil
		}
		m.cursor[m.tab] -= m.bodyHeight() / 2
	case keyFilter:
		if m.tab == 0 {
			m.filter = (m.filter + 1) % 4
		} else if m.tab == 2 {
			m.prFilter = (m.prFilter + 1) % 3
		}
	case "h": // tasks: collapse the fold under the cursor (tree navigation)
		if m.tab == 0 && !m.rightFocus {
			if id := m.selID(); id != "" {
				m.collapsed[id] = true
			}
		}
	case "l": // tasks: expand the fold under the cursor (tree navigation)
		if m.tab == 0 && !m.rightFocus {
			delete(m.collapsed, m.selID())
		}
	case keyStartS: // agents: Start/Stop toggle — start if down, stop if running
		if m.tab == 1 {
			return m.agentStartStop()
		}
	case keyAttach: // agents/tasks/prs: attach to the live tmux session
		if m.tab == 0 {
			// Attach to whoever is working the selected task — the row you are looking at names
			// the work, so it should reach the agent doing it without a detour via the Agents tab.
			a, ok := m.agentOnTask(m.selID())
			if !ok {
				m.flash = "no agent is working " + m.selID()
				return nil
			}
			return m.attachOrArm(a, "'"+keyStartS+"' on the Agents tab")
		}
		if m.tab == 1 {
			if a, ok := m.selAgent(); ok {
				return m.attachOrArm(a, "'"+keyStartS+"'")
			}
		}
		if m.tab == 2 {
			// Same reasoning as tasks: the PR names the work, so attach reaches its author
			// without a detour via the Agents tab.
			a, ok := m.agentOnPR(m.selID())
			if !ok {
				m.flash = "no agent is working " + m.selID()
				return nil
			}
			return m.attachOrArm(a, "'"+keyStartS+"' on the Agents tab")
		}
	case keyMerge: // agents: milestone PR · prs: merge (the human gate)
		if m.tab == 1 {
			if a, ok := m.selAgent(); ok {
				m.openMilestoneChoice(a.Name)
			}
			return nil
		}
		if m.tab == 2 && m.selID() != "" {
			if !m.selPRApproved() {
				m.openApproveMergeChoice(m.selID())
				return nil
			}
			id := m.selID()
			m.markMerging(id) // show "merging" on the row at once, before the hub confirms
			return m.mergeCmd(id)
		}
	case keyNew: // new task (tasks) / new agent (agents) / new meeting (meeting)
		if m.tab == 0 {
			m.openTaskForm(false, api.Task{})
			return nil
		} else if m.tab == 1 { // agents: pick the role, then auto-name after a dwarf
			m.openNewAgentChoice()
			return nil
		} else if m.tab == 4 { // meeting: clear the shared history and start fresh
			m.openNewMeetingChoice()
			return nil
		}
	case keyEdit: // edit what is selected, each tab in its own natural way
		if m.tab == 0 && m.selID() != "" && m.cl != nil {
			return editFetchCmd(m.cl, m.selID()) // a task's fields: pre-edit sync, then the form
		}
		if m.tab == 1 { // an agent's own workspace, in $EDITOR
			if dir := m.agentWorkspacePath(m.selID()); dir != "" {
				return m.editorAtCmd(dir)
			}
			return nil
		}
		if m.tab == 2 { // a PR's review checkout, in $EDITOR
			if id := m.selID(); id != "" && m.cl != nil {
				return m.openEditorCmd(id)
			}
		}
	case keyOpen: // agents/prs: open the row's worktree in a shell, the same one ENTER on its path gives
		if m.tab == 1 || m.tab == 2 {
			if dir := m.selWorktree(); dir != "" {
				return tea.ExecProcess(shellAt(dir), resumed)
			}
			if id := m.selID(); id != "" {
				m.flash = "no worktree for " + id // a PR whose agent is gone has none left
			}
			return nil
		}
	case keyComment: // tasks: comment on the selected task
		if m.tab == 0 && m.selID() != "" {
			m.openInput(inputComment, "comment on "+m.selID()+": ")
			return textinput.Blink
		}
	case keyBrief: // tasks: brief a planner · agents: reBuild the agent image
		if m.tab == 0 && m.selID() != "" {
			m.openBriefChoice(m.selID())
			return nil
		}
		if m.tab == 1 {
			if a, ok := m.selAgent(); ok {
				m.openRebuildChoice(a.Name)
			}
			return nil
		}
	case keyStats: // agents: the fleet's memory use against its limits (a view)
		if m.tab == 1 {
			return m.statsCmd()
		}
	case keyOptions: // agents: the selected agent's options · tasks: reopen a closed task
		if m.tab == 1 {
			if a, ok := m.selAgent(); ok {
				m.openAgentOptionsForm(a.Name, a.Memory)
				return nil
			}
		}
		if m.tab == 0 && m.taskReopenable() {
			m.openTaskReopenForm(m.selID())
			return nil
		}
	case keyDelete: // tasks: scrap · agents: delete (or remove an orphan) · prs: scrap · repos: forget
		if m.tab == 0 && m.selID() != "" {
			m.openScrapChoice(m.selID())
			return nil
		}
		if m.tab == 2 && m.selID() != "" {
			m.openScrapPRChoice(m.selID())
			return nil
		}
		if m.tab == 1 && m.selID() != "" {
			if m.isOrphan(m.selID()) {
				m.openRemoveOrphanChoice(m.selID())
			} else {
				m.openDeleteChoice(m.selID())
			}
			return nil
		}
		if m.tab == 3 && m.selID() != "" {
			m.openForgetChoice(m.selID(), m.repoName(m.selID()))
			return nil
		}
	case keyTell: // tell the selected agent (agents) / show linked task (prs)
		if m.tab == 1 && m.selID() != "" && !m.isOrphan(m.selID()) {
			m.openInput(inputTell, "tell "+m.selID()+": ")
			return textinput.Blink
		} else if m.tab == 2 {
			if d := m.prDetail; d.PR.ID == m.selID() && d.Task.ID != "" {
				m.openTaskModal(d.Task)
			}
			return nil
		}
	case keyLint: // prs: run the quality gate against the PR's worktree
		if m.tab == 2 {
			if id := m.selID(); id != "" && m.cl != nil {
				return m.lintCmd(id)
			}
		}
	case keyReject: // prs: reject a PR · tasks: reject a proposal · agents: rebase (R = reBase) · meeting: remove a member
		if m.tab == 2 && m.selID() != "" {
			m.openRejectForm(m.selID())
			return nil
		}
		if m.tab == 0 && m.taskGated() {
			m.openTaskRejectForm(m.selID())
			return nil
		}
		if m.tab == 1 && m.selID() != "" && !m.isOrphan(m.selID()) {
			return m.rebaseAgentCmd(m.selID())
		}
		if m.tab == 4 {
			m.openRemoveMemberChoice()
			return nil
		}
	case keyVerify: // prs: verify — materialize the PR into the review workspace + shell in
		if m.tab == 2 {
			if id := m.selID(); id != "" && m.cl != nil {
				return m.verifyCmd(id)
			}
		}
	case keyApprove: // approve, the human gate: a PR (prs) / a planner-proposed task (tasks) / meeting: add a member
		if m.tab == 2 && m.selID() != "" { // approve the PR yourself, so it can be merged
			return m.action(func(id string) error { return m.cl.ApprovePR(id) })
		}
		if m.tab == 0 && m.taskGated() {
			id := m.selID()
			if pending := m.pendingBelow(id); pending > 0 { // ask how far the verdict carries
				m.openApproveChoice(id, pending)
				return nil
			}
			m.flash = "approving " + id + "…"
			return approveTaskCmd(m.cl, id, false, m.priorityAfterApprove(id))
		}
		if m.tab == 4 {
			m.openAddMemberChoice()
			return nil
		}
	case keyReview: // prs: hand the PR to a reviewer agent
		if m.tab == 2 && m.selID() != "" {
			m.openReviewForm(m.selID())
			return nil
		}
	case keyPriority: // set the selected task's priority (shift = a modifying action)
		if m.tab == 0 && m.selID() != "" {
			m.openPriorityChoice(m.selID())
			return nil
		}
	case keyUnassign: // tasks: release the selected task back to the backlog
		if m.tab == 0 && m.selID() != "" {
			return m.unassignTaskCmd(m.selID())
		}
	case keyWhyNext: // tasks: what the assigner would hand out next, and why not everything else
		if m.tab == 0 {
			return m.whyNextCmd()
		}
	case keyClose: // tasks: close the selected task (mark it done) · agents: clear a full context
		if m.tab == 0 && m.selID() != "" {
			if pr := m.attachedOpenPR(m.selID()); pr != "" { // prompt to discard its PR too
				m.openCloseChoice(m.selID(), pr)
				return nil
			}
			return m.closeTaskCmd(m.selID())
		}
		if m.tab == 1 && m.selID() != "" && !m.isOrphan(m.selID()) {
			m.openClearContextChoice(m.selID())
			return nil
		}
	case "enter":
		if m.tab == 4 { // Chat: open the multiline composer in the main pane
			return m.startComposing()
		}
		if m.rightFocus { // act on the focused detail item
			if it, ok := m.focusedItem(); ok {
				switch it.kind {
				case "view": // switch the big content pane
					if m.tab == 1 { // Agents: live screen ⇄ pod info ⇄ liveness probe
						if m.agentView == it.value { // selecting the shown view returns to the screen
							m.agentView = "screen"
							return nil
						}
						m.agentView = it.value
						if m.cl == nil {
							return nil
						}
						if it.value == "diag" {
							return diagFetchCmd(m.cl, m.selID())
						}
						return podFetchCmd(m.cl, m.selID())
					}
					m.prView = it.value // PRs: diff ⇄ lint
					m.detail.Resize(m.detail.Height, len(m.prContentLines()))
				case "path": // open a shell in the workspace
					return tea.ExecProcess(shellAt(it.value), resumed)
				case "url": // e.g. a GitHub issue: no browser in the pod's TUI, so copy it instead
					_ = clipboard.WriteAll(it.value)
					m.flash = "copied URL: " + it.value
				default: // cross-reference: open its details modal
					m.openItemModal(it.kind, it.value)
				}
			}
			return nil
		}
		if m.tab == 3 { // Repos: enter switches to the selected repo
			if tag := m.selID(); tag != "" {
				for _, p := range m.state.Projects {
					if p.Tag == tag {
						return m.switchRepo(p.Path)
					}
				}
			}
			return nil
		}
		if m.selID() != "" { // open the full-screen detail modal
			m.modal = true
			m.detail.SetHeight(modalContentHeight(m.h))
			m.detail.SetTotal(len(m.modalLines()))
			m.detail.ScrollTop()
			return nil
		}
	case keyDetail: // toggle the detail pane (full-width selector when hidden)
		m.hideDetail = !m.hideDetail
		if m.hideDetail {
			m.rightFocus = false // can't focus a hidden pane
		}
	case keyRefresh:
		m.reclamp()
		if m.tab == 0 && m.selID() != "" && m.cl != nil { // also pull the task's comments fresh
			return tea.Batch(m.refreshCmd(), refreshTaskCommentsCmd(m.cl, m.selID()))
		}
		return m.refreshCmd()
	case keyRepo: // switch the active repo/project (lowercase = harmless navigation)
		m.openSwitcher()
		return nil
	case keyConfig: // edit the current repo's .sindri/config.yaml in a form
		return m.repoConfigCmd()
	case keyColor: // repos: pick the selected repo's display colour
		if m.tab == 3 && m.selID() != "" {
			m.openColorChoice(m.selID())
			return nil
		}
	case keyScopeTog: // agents/prs: toggle the TUI-wide scope between the active repo and all repos
		if m.tab == 1 || m.tab == 2 {
			m.scopeRepo = !m.scopeRepo
			// Both Agents and PRs re-filter, so reset both their cursors (reclamp
			// keeps them valid, but the lists change out from under the old position).
			m.cursor[1], m.cursor[2] = 0, 0
			if m.scopeRepo {
				m.flash = "scope: this repo"
			} else {
				m.flash = "scope: all repos"
			}
		}
	}
	m.reclamp()
	cmd := m.syncDetail()
	if m.tab != oldTab { // changing tabs: drop right-column focus, auto-refresh
		m.rightFocus, m.rightCursor = false, 0
		cmds := []tea.Cmd{cmd, m.refreshCmd()}
		if m.tab == 4 && m.cl != nil { // entered Chat: register presence at once (don't wait for the tick)
			cmds = append(cmds, chatHeartbeatCmd(m.cl))
		}
		return tea.Batch(cmds...)
	}
	return cmd
}

// resumed asks the loop to repaint after an interactive child exits. Its exit error is dropped:
// nothing here can act on a shell that exited non-zero, and a modal about it is only noise.
func resumed(error) tea.Msg { return resumedMsg{} }
