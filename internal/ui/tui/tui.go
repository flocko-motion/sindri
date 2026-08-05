// package: tui / tui
// type:    ui (a thin hub client)
// job:     the dashboard shell — model, update loop, and the full-height
// master-detail View that composes the generic components
// (component_*.go) around the per-tab content (tasks.go/agents.go/
// prs.go). Live over /events; all derivation comes from the hub.
// limits:  no domain logic; hub client only; refuses to start without a hub.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/ui/tui/scroll"
)

const (
	filterOpen = iota
	filterClosed
	filterAll
	filterActive
)

var filterNames = [...]string{"open", "closed", "all", "active"}

// activeWindow is how recently a task must have changed to count as "active" alongside every
// open task — wide enough that a task closed just before you glanced over doesn't vanish.
const activeWindow = 2 * time.Hour

// inputMode is the active text-input modal (none = normal navigation).
type inputMode int

const (
	inputNone inputMode = iota
	inputTell
	inputComment
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
	hideDetail bool              // § force-hides the detail pane (else shown when wide enough)
	scopeRepo  bool              // TUI-wide global↔repo scope (default repo): Agents/PRs narrow to the active repo when true. Tasks is always repo-scoped regardless.

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

	composing  bool           // Chat tab: the multiline composer is open in the main pane
	composer   textarea.Model // multiline chat compose (a single line can't hold deep talk)
	modal      bool           // detail modal (full-screen) is open
	choice     choiceModalState
	form       formState // active fill-in form (new/edit task)
	flash      string    // transient status (e.g. "copied"), cleared on next key
	errText    string    // when set, the error modal is shown (any key dismisses)
	noticeText string    // when set, a startup warning modal is shown (any key dismisses)
}

// detailMinWidth is the floor below which the side detail column cannot coexist with the main
// content. Only that secondary column is ever gated on width; the main pane never is.
const detailMinWidth = 80

// wide reports whether there's room for a side detail column at all.
func (m model) wide() bool { return m.w >= detailMinWidth }

// showDetail reports whether the right detail column is shown: on by default when
// there's room, off when the user hides it with §. Never hides the main pane.
func (m model) showDetail() bool { return m.wide() && !m.hideDetail }

func newModel(cl *client.HTTP, ch <-chan hub.BoardState, root string) model {
	// A default size renders a frame immediately: some terminals report theirs late, or as 0×0,
	// and the view would otherwise stick on "loading".
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
		// A shrink leaves cells of the larger frame behind, so the new one draws over
		// leftovers. Clear before repainting.
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
		// An agent's screen and log go stale between hub notifications, so the Agents tab polls;
		// the state poll cascades to those refetches via polledMsg.
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
			m.agentPane = KeepColour(msg.text) // another program's screen: text + colour only
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
	case openPlanFormMsg: // "new… → plan" chosen — ask what to plan
		m.openPlanForm(string(msg))
		return m, nil
	case openTaskPlanFormMsg: // a planner was chosen for a task — ask what to add, then hand it over
		m.openTaskPlanForm(msg.planner, msg.task)
		return m, nil
	case editorReadyMsg: // PR materialized — open the user's editor on the review workspace
		ed := editorAt(string(msg))
		if ed == nil {
			m.errText = "no editor found — set $EDITOR (or $VISUAL) to the one you want."
			return m, nil
		}
		return m, tea.ExecProcess(ed, resumed)
	case prMsg:
		m.prDetail = msg.d
		// The diff arrives long after syncDetail sized the viewport to "(loading…)", so resize
		// now or it renders against a stale one-line window.
		m.reclamp()
	case taskMsg:
		m.taskDetail = msg.t
	case repoConfigMsg:
		if msg.err != nil {
			m.errText = msg.err.Error()
		} else {
			m.openRepoConfigForm(msg.d)
		}
	case formFailedMsg: // the form keeps the typing; the reason goes in its own footer
		m.form.submitting = false
		m.form.err = msg.err.Error()
		return m, nil
	case formAppliedMsg:
		m.form.active, m.form.submitting = false, false
		if msg.inner == nil {
			return m, nil
		}
		inner := msg.inner // forward the apply's own message, so the board still updates
		return m, func() tea.Msg { return inner }
	case errModalMsg:
		m.errText = msg.err.Error() // shown over everything; a composing draft stays open beneath it
	case chatSentMsg:
		m.composer.Reset() // the hub has it; the kept draft is no longer needed
		m.flash = "sent"
	case chatFailedMsg: // reopen on the draft, so the reason and the text are in front of you
		m.errText = msg.err.Error()
		m.composer.SetValue(msg.draft)
		m.composing = true
		return m, m.composer.Focus()
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
		labels[i] = fmt.Sprintf("%d %s", m.tabCount(s), s.Title)
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
	// Agents/PRs always render their main pane — it is the point of the tab. Each body drops
	// only the right detail column when the terminal is narrow or § hid it.
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
		foot = dimStyle.Render(padTrunc("ctrl+s send · enter newline (sends a /command) · esc cancel", m.w)) + "\n" +
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
