// package: tui / tui
// type:    ui (a thin hub client)
// job:     the dashboard shell — model, update loop, and the full-height
//          master-detail View that composes the generic components
//          (component_*.go) around the per-tab content (tasks.go/agents.go/
//          prs.go). Live over /events; all derivation comes from the hub.
// limits:  no domain logic; hub client only; refuses to start without a hub.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/hub/client"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/ui/tui/scroll"
)

const (
	filterOpen = iota
	filterClosed
	filterAll
)

var filterNames = [...]string{"open", "closed", "all"}

// inputMode is the active text-input modal (none = normal navigation).
type inputMode int

const (
	inputNone inputMode = iota
	inputTell
)

type model struct {
	cl     *client.HTTP
	ch     <-chan hub.BoardState
	cancel context.CancelFunc // cancels the current /events subscription (re-created on repo switch)
	gen    int                // subscription generation; bumped on switch so stale /events msgs are ignored
	root   string             // the selected repo — scopes the Tasks tab and container names
	state  hub.BoardState
	err    error
	w, h   int

	tab    int
	cursor [5]int // one per section (Tasks/Agents/PRs/Repos/Chat)
	list   scroll.Viewport
	detail scroll.Viewport

	filter     int // Tasks tab: open/closed/all
	prFilter   int // PRs tab: unmerged/merged/all (default hides merged)
	collapsed  map[string]bool
	merging    map[string]bool   // PR ids the user just triggered a merge on — shown as a transient "merging" on the row until the hub confirms
	busy       map[string]string // task ids the user just triggered a close/scrap on → the transient verb ("closing"/"deleting") shown on the row until the hub confirms
	hideDetail bool            // § force-hides the detail pane (else shown when wide enough)
	scopeRepo  bool            // TUI-wide global↔repo scope (default repo): Agents/PRs narrow to the active repo when true. Tasks is always repo-scoped regardless.

	rightFocus  bool // detail (right) column has focus (h/l switch; j/k move within)
	rightCursor int  // focused actionable item in the right column

	detailKey    string
	agentLog     []store.Event
	agentPane    string           // captured tmux screen of the selected agent (live)
	agentView    string           // Agents main pane: "screen" (tmux, default) | "pod" (podman info)
	agentPod     string           // fetched podman pod-info for the selected agent
	agentClients []hub.ClientView // dial-ins attached to the selected agent's session
	prDetail     hub.PRDetail
	prView       string // which content the PR big pane shows: "diff" (default) | "lint"
	reviewPrompt string // editable default review instruction (from the hub)
	taskDetail   store.Task
	quit         bool

	modalOverride      []string // when set, the detail modal shows these instead of the tab detail
	modalOverrideTitle string

	mode        inputMode // active text-input modal
	input       textinput.Model
	inputTarget string // selection captured when the modal opened

	composing bool          // Chat tab: the multiline composer is open in the main pane
	composer  textarea.Model // multiline chat compose (a single line can't hold deep talk)
	modal       bool   // detail modal (full-screen) is open
	choice      choiceModalState
	form        formState // active fill-in form (new/edit task)
	flash       string    // transient status (e.g. "copied"), cleared on next key
	errText     string    // when set, the error modal is shown (any key dismisses)
	noticeText  string    // when set, a startup warning modal is shown (any key dismisses)
}

// detailMinWidth is the floor below which a side detail column can't usefully
// coexist with the main content — genuinely too little room, not a preference. Above
// it the user controls the column with § (it shows by default). The main pane (task
// list / live screen / diff) is NEVER gated on width; only this secondary column is.
const detailMinWidth = 80

// wide reports whether there's room for a side detail column at all.
func (m model) wide() bool { return m.w >= detailMinWidth }

// showDetail reports whether the right detail column is shown: on by default when
// there's room, off when the user hides it with §. Never hides the main pane.
func (m model) showDetail() bool { return m.wide() && !m.hideDetail }

func newModel(cl *client.HTTP, ch <-chan hub.BoardState, root string) model {
	// Default to a sane size so a frame renders immediately — the real size
	// arrives via WindowSizeMsg and resizes. (Some terminals send the initial
	// size late or as 0×0; without a default the view would stick on "loading".)
	in := textinput.New()
	in.CharLimit = 0 // no limit — a chat message (or tell) must never be silently truncated on send
	ta := textarea.New()
	ta.CharLimit = 0 // the hub enforces the length cap (with feedback); never clip silently here
	ta.Placeholder = "Type a message to the meeting room…"
	ta.ShowLineNumbers = false
	m := model{cl: cl, ch: ch, root: root, collapsed: map[string]bool{}, merging: map[string]bool{}, busy: map[string]string{}, scopeRepo: true, w: 80, h: 24, input: in, composer: ta}
	m.reclamp()
	return m
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{waitForState(m.ch, m.gen), tickCmd()}
	if m.cl != nil { // load the editable default review prompt
		cl := m.cl
		cmds = append(cmds, func() tea.Msg {
			p, err := cl.ReviewPrompt()
			if err != nil {
				return nil
			}
			return reviewPromptMsg(p)
		})
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 && msg.Height > 0 { // ignore bogus 0×0 (keeps the default)
			m.w, m.h = msg.Width, msg.Height
		}
		if m.mode != inputNone { // keep the compose field sized to the new width
			m.resizeInput()
		}
		if m.composing { // keep the multiline composer sized to the new dimensions
			m.sizeComposer()
		}
		m.reclamp()
		if m.modal {
			m.detail.SetHeight(modalContentHeight(m.h))
		}
		// A resize — a shrink especially — leaves stale cells from the old, larger
		// frame in the alt-screen buffer, so the new frame renders over leftovers
		// (the "ugly resize"). Force a full clear+repaint so it comes up clean.
		return m, tea.ClearScreen
	case stateMsg:
		if msg.gen != m.gen { // a snapshot from a stream abandoned by a repo switch
			return m, nil
		}
		m.state = msg.st
		m.reconcileMerging()
		m.reconcileBusy()
		m.reclamp()
		return m, tea.Batch(waitForState(m.ch, m.gen), m.syncDetail(), m.agentLiveCmds())
	case polledMsg: // an auto-refresh poll — update the board, don't touch the SSE waiter
		m.state = hub.BoardState(msg)
		m.reconcileMerging()
		m.reconcileBusy()
		m.reclamp()
		return m, tea.Batch(m.syncDetail(), m.agentLiveCmds())
	case approveMergeMsg: // "approve & merge" confirmed — mark the row merging, then run it
		m.markMerging(msg.id)
		return m, m.approveMergeCmd(msg.id)
	case mergeDoneMsg:
		return m.mergeDone(msg)
	case taskOpMsg: // close/scrap confirmed — mark the transient verb, then run the op
		if m.busy == nil {
			m.busy = map[string]string{}
		}
		m.busy[msg.id] = msg.verb
		return m, msg.run
	case taskOpDoneMsg:
		return m.taskOpDone(msg)
	case tickMsg:
		// Live agent state (status), screen, and log go stale between hub
		// notifications; while on the Agents tab, poll every few seconds. The
		// state poll cascades to log+screen refetches via polledMsg.
		cmds := []tea.Cmd{tickCmd()}
		if m.tab == 1 && m.cl != nil {
			cmds = append(cmds, pollStateCmd(m.cl))
		}
		if m.tab == 4 && m.cl != nil { // Chat tab open: keep the room unlocked (presence)
			cmds = append(cmds, chatHeartbeatCmd(m.cl))
		}
		return m, tea.Batch(cmds...)
	case logMsg:
		if msg.key == m.selID() { // ignore a stale fetch from a prior selection
			m.agentLog = msg.evs
		}
	case paneMsg:
		if msg.agent == m.selID() { // ignore a stale capture from a prior selection
			m.agentPane = msg.text
		}
	case agentPodMsg:
		if msg.agent == m.selID() {
			m.agentPod = msg.text
		}
	case clientsMsg:
		if msg.agent == m.selID() { // ignore a stale fetch from a prior selection
			m.agentClients = msg.clients
		}
	case prLintMsg:
		if msg.pr == m.selID() { // store the result, switch to the lint view, focus it
			m.prDetail.Lint = msg.text
			m.prView = "lint"
			m.rightFocus = true
			m.rightCursor = m.viewCursor("lint")
			m.detail.Resize(m.detail.Height, len(m.prContentLines()))
		}
	case reviewPromptMsg:
		m.reviewPrompt = string(msg)
	case resumedMsg: // an interactive child (attach/shell) exited — force a clean repaint (see resumedMsg)
		return m, tea.ClearScreen
	case reviewReadyMsg: // PR materialized — drop into a shell in the review workspace
		return m, tea.ExecProcess(shellAt(string(msg)), resumed)
	case prMsg:
		m.prDetail = msg.d
		// The diff arrives async, well after syncDetail sized the viewport to the
		// "(loading…)" placeholder. Resize it to the real content now, or the diff
		// renders against a stale 1-line window (showing only its header) until some
		// later event happens to reclamp.
		m.reclamp()
	case taskMsg:
		m.taskDetail = msg.t
	case repoConfigMsg:
		if msg.err != nil {
			m.errText = msg.err.Error()
		} else {
			m.openRepoConfigForm(msg.d)
		}
	case errModalMsg:
		m.errText = msg.err.Error() // shown over everything; a composing draft stays open beneath it
	case chatSentMsg:
		m.composer.Reset() // sent OK — clear + close the composer
		m.composing = false
		m.composer.Blur()
	case openEditMsg: // pre-edit sync completed — open the form from the fresh task
		m.openTaskForm(true, msg.t)
	case errMsg:
		if msg.gen != m.gen { // the close of a stream abandoned by a repo switch — not fatal
			return m, nil
		}
		m.err = msg.err
		return m, tea.Quit
	case switchRepoMsg:
		return m, m.switchRepo(string(msg))
	case tea.KeyMsg:
		if m.errText != "" { // any key dismisses the error modal
			m.errText = ""
			return m, nil
		}
		if m.noticeText != "" { // any key dismisses the startup notice
			m.noticeText = ""
			return m, nil
		}
		if m.form.active {
			return m, m.form.update(msg)
		}
		if m.composing { // Chat tab: the multiline composer owns the keyboard while open
			return m.updateComposer(msg)
		}
		if m.mode != inputNone {
			return m.updateInput(msg)
		}
		if m.choice.active {
			return m.updateChoice(msg)
		}
		if m.modal {
			return m.updateModal(msg)
		}
		cmd := m.onKey(msg.String())
		if m.quit {
			return m, tea.Quit
		}
		return m, cmd
	}
	return m, nil
}

// modalTitle labels the detail modal for the current selection.

// onKey applies a key (by its string form) — shared by the live loop and the
// headless Screenshot harness. Mutates the model; returns an optional cmd.
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
		m.tab = (m.tab + 1) % len(hub.Sections)
	case "shift+tab", "[": // switch tabs back ([ mirrors shift+tab)
		m.tab = (m.tab - 1 + len(hub.Sections)) % len(hub.Sections)
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
		for i := 0; i < detailScrollStep; i++ {
			m.detail.ScrollDown()
		}
		return nil
	case "K": // scroll the detail pane up
		for i := 0; i < detailScrollStep; i++ {
			m.detail.ScrollUp()
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
			m.filter = (m.filter + 1) % 3
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
	case keyAttachAp: // agents: attach to the live tmux session · prs: approve (the human gate)
		if m.tab == 1 {
			if a, ok := m.selAgent(); ok {
				if a.Status == "down" {
					m.errText = "agent " + a.Name + " is down — start it first ('S') before attaching"
					return nil
				}
				if m.cl != nil {
					return tea.ExecProcess(attachCmd(m.agentContainer(a), a.Name), resumed)
				}
			}
		}
		if m.tab == 2 && m.selID() != "" { // approve the PR yourself, so it can be merged
			return m.action(func(id string) error { return m.cl.ApprovePR(id) })
		}
	case keyMerge: // prs: merge (the human gate) — if it isn't approved, offer to approve first
		if m.tab == 2 && m.selID() != "" {
			if !m.selPRApproved() {
				m.openApproveMergeChoice(m.selID())
				return nil
			}
			id := m.selID()
			m.markMerging(id) // show "merging" on the row at once, before the hub confirms
			return m.mergeCmd(id)
		}
	case keyNew: // new task (tasks) / new agent (agents)
		if m.tab == 0 {
			m.openTaskForm(false, store.Task{})
			return nil
		} else if m.tab == 1 { // agents: pick the role, then auto-name after a dwarf
			m.openNewAgentChoice()
			return nil
		}
	case keyEdit: // edit: the selected task's fields (tasks) / the agent's memory limit (agents)
		if m.tab == 0 && m.selID() != "" && m.cl != nil {
			return editFetchCmd(m.cl, m.selID()) // pre-edit sync: fetch fresh, then open the form
		}
		if m.tab == 1 {
			if a, ok := m.selAgent(); ok {
				m.openMemoryForm(a.Name, a.Memory)
				return nil
			}
		}
	case keyDelete: // tasks: scrap · agents: delete (or remove an orphan) · repos: forget
		if m.tab == 0 && m.selID() != "" {
			m.openScrapChoice(m.selID())
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
	case keyReject: // prs: reject a PR · tasks: reject a proposal · agents: rebase (R = reBase)
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
	case keyVerify: // prs: verify — materialize the PR into the review workspace + shell in
		if m.tab == 2 {
			if id := m.selID(); id != "" && m.cl != nil {
				return m.verifyCmd(id)
			}
		}
	case keyApprove: // prs: request an agentic review · tasks: approve a planner-proposed task
		if m.tab == 2 && m.selID() != "" {
			m.openReviewForm(m.selID())
			return nil
		}
		if m.tab == 0 && m.taskGated() {
			return m.approveTaskCmd(m.selID())
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
	case keyClose: // tasks: close the selected task (mark it done)
		if m.tab == 0 && m.selID() != "" {
			if pr := m.attachedOpenPR(m.selID()); pr != "" { // prompt to discard its PR too
				m.openCloseChoice(m.selID(), pr)
				return nil
			}
			return m.closeTaskCmd(m.selID())
		}
	case "enter":
		if m.tab == 4 { // Chat: open the multiline composer in the main pane
			return m.startComposing()
		}
		if m.rightFocus { // act on the focused detail item
			if it, ok := m.focusedItem(); ok {
				switch it.kind {
				case "view": // switch the big content pane
					if m.tab == 1 { // Agents: toggle live screen ⇄ pod info
						if m.agentView == "pod" {
							m.agentView = "screen"
							return nil
						}
						m.agentView = "pod"
						if m.cl != nil {
							return podFetchCmd(m.cl, m.selID())
						}
						return nil
					}
					m.prView = it.value // PRs: diff ⇄ lint
					m.detail.Resize(m.detail.Height, len(m.prContentLines()))
				case "path": // open a shell in the workspace
					return tea.ExecProcess(shellAt(it.value), resumed)
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

// resumed is the tea.ExecProcess callback for every interactive child (tmux
// attach, workspace shell): on exit it asks the loop to repaint via resumedMsg. The
// child's own exit error is intentionally dropped — there's nothing the dashboard
// can do about a shell that exited non-zero, and a stray modal would just be noise.
func resumed(error) tea.Msg { return resumedMsg{} }

// View composes the full-height frame: tab strip, master-detail body, footer.
func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("hub connection lost: %v\n", m.err)
	}
	if m.w == 0 || m.h == 0 {
		return "loading…"
	}
	labels := make([]string, len(hub.Sections))
	for i, s := range hub.Sections {
		labels[i] = fmt.Sprintf("%d %s", s.Count(m.state), s.Title)
	}
	// Modals take over the whole screen.
	if m.errText != "" {
		return errModal(m.errText, m.w, m.h)
	}
	if m.noticeText != "" {
		return warnModal(m.noticeText, m.w, m.h)
	}
	if m.form.active {
		return m.form.view(m.w, m.h)
	}
	if m.choice.active {
		return choiceModal(m.choice, m.w, m.h)
	}
	if m.modal {
		title := m.modalTitle()
		if m.modalOverride != nil { // e.g. the task modal opened from the PRs tab
			title = m.modalOverrideTitle
		}
		return modal(title, m.modalLines(), m.detail, m.w, m.h)
	}
	repoName, repoTag := m.currentRepo()
	top := headerBar(labels, m.tab, m.w, repoName, repoTag, m.repoColorIdx(repoTag))
	var body string
	// Agents/PRs always render their MAIN pane (live tmux screen / diff) — it's the
	// point of the tab and must never be hidden. agentsBody/prBody handle a narrow or
	// §-hidden terminal internally (they drop only the RIGHT detail column, keeping
	// the list + main pane full-width).
	if m.tab == 1 {
		body = m.agentsBody()
	} else if m.tab == 2 {
		body = m.prBody()
	} else if m.tab == 4 {
		body = m.chatBody()
	} else if m.showDetail() {
		left := pane(rowTexts(m.rows()), m.list, m.leftWidth(), m.cursor[m.tab])
		dlines, dhl := m.wrappedDetail()
		right := pane(dlines, m.detail, m.detailWidth(), dhl)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, divider(m.bodyHeight()), right)
	} else {
		// Narrow terminal: selector full-width; detail is ENTER-only.
		body = pane(rowTexts(m.rows()), m.list, m.w, m.cursor[m.tab])
	}
	var foot string
	switch {
	case m.mode != inputNone:
		foot = dimStyle.Render(padTrunc("enter submit · esc cancel", m.w)) + "\n" + m.input.View()
	case m.composing:
		foot = dimStyle.Render(padTrunc("ctrl+s send · esc cancel · enter newline", m.w)) + "\n" +
			dimStyle.Render(padTrunc("composing to the meeting room…", m.w))
	default:
		global := m.footerFor(scopeGlobal)
		if m.flash != "" {
			global = m.flash
		}
		foot = footer(global, m.contextFooter(), m.w)
	}
	return strings.Join([]string{top, body, foot}, "\n")
}
