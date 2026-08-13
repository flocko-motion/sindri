// package: tui / messages
// type:    ui (Bubble Tea message types)
// job:     the messages the update loop reacts to — board snapshots, lazily
// fetched detail (log/pr/task/pane/pod), and the poll/error signals —
// plus the few timing constants that govern live updates.
// limits:  message type definitions only; the loop that reacts to them is in
// tui.go (-> Update).
package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flo-at/sindri/internal/api"
)

// stateMsg is a board snapshot from /events; gen lets a stream abandoned by a repo switch be ignored.
type stateMsg struct {
	st  api.BoardState
	gen int
}
type logMsg struct {
	key string
	evs []api.Event
}
type prMsg struct {
	key string
	d   api.PRDetail
}
type taskMsg struct {
	key string
	t   api.Task
}
type runMsg struct {
	key string
	d   api.RunDetail
}
type paneMsg struct {
	agent string
	text  string
}

// agentCreatedMsg names a freshly registered agent, so Update can start it — registration and
// launch are two hub calls, and the second one needs to report for itself.
type agentCreatedMsg string

// launchedMsg is a finished launch: the log either way, and the error if it failed.
type launchedMsg struct {
	name string
	log  string
	err  error
}

// agentDiagMsg carries the liveness-probe explanation for an agent.
type agentDiagMsg struct {
	agent string
	text  string
}

type agentPodMsg struct {
	agent string
	text  string
}
type clientsMsg struct {
	agent   string
	clients []api.ClientView
}
type prLintMsg struct {
	pr   string
	text string
}

// milestoneMsg names the PR that was opened. The board itself arrives on the state stream, as it
// does after a merge, so it is not refetched here.
type milestoneMsg string

// noticeMsg is read-only text to put in front of the user until they dismiss it.
type noticeMsg string

// rebuiltMsg carries the build log, and the error if the build failed — the log is worth showing
// either way, so it is not discarded on success.
type rebuiltMsg struct {
	name string
	log  string
	err  error
}

// statsMsg is the fleet's sampled memory use.
type statsMsg api.StatsReport

type reviewPromptMsg string
type reviewReadyMsg string // the review-workspace path to open a shell in
type editorReadyMsg string // the review-workspace path to open the user's editor on

// openPlanFormMsg routes the picked planner through Update: a choice's apply returns a cmd, but the
// form is model state.
type openPlanFormMsg string

// openTaskPlanFormMsg carries the chosen planner and the task it works up, for the same reason
// openPlanFormMsg exists: a chooser's apply returns a cmd, and opening a form is Update's to do.
type openTaskPlanFormMsg struct{ planner, task string }

// approveMergeMsg routes the intent through Update so the "merging" marker renders before the async work.
type approveMergeMsg struct{ id string }

// mergeDoneMsg reports a finished merge; either way the transient "merging" marker is cleared.
type mergeDoneMsg struct {
	id    string
	state api.BoardState
	err   error
}

// taskOpMsg routes a close/scrap through Update — a choice's apply can't mutate the returned
// model — so the transient verb renders before run fires.
type taskOpMsg struct {
	id   string
	verb string
	run  tea.Cmd
}

// taskOpDoneMsg reports a finished close/scrap; either way the transient verb is cleared. then is a
// follow-up the op earned (-> afterTaskOp): it runs after the fresh board has been applied, so what it
// opens sees the state the op produced.
type taskOpDoneMsg struct {
	id    string
	state api.BoardState
	err   error
	then  tea.Cmd
}

// openPriorityChoiceMsg routes the priority picker through Update — a cmd cannot open a modal itself —
// which is how an approve hands on to the rating that actually releases the task.
type openPriorityChoiceMsg string

// openPriorityScopeMsg carries the picked P-code to the scope step, for the same reason.
type openPriorityScopeMsg struct{ id, code string }

// openApproveAfterPriorityMsg is the mirror of openPriorityChoiceMsg: a rating landed on a task
// still awaiting a verdict, so the gate it did NOT open is offered next.
type openApproveAfterPriorityMsg string

// paneLines is how many rows of an agent's tmux scrollback the detail shows.
const paneLines = 200

type errMsg struct { // fatal: hub connection lost (unless a stale generation)
	err error
	gen int
}
type errModalMsg struct{ err error } // non-fatal: show the error modal
type chatSentMsg struct{}            // a chat compose sent OK — clear the kept draft

// formFailedMsg is a form submit the hub refused: shown in the form's own footer, with every field
// left as typed so the reported value can be fixed and sent again.
type formFailedMsg struct{ err error }

// formAppliedMsg is a form submit the hub took. inner is the apply's own message (a fresh board,
// usually), forwarded so accepting a form still updates what it changed.
type formAppliedMsg struct{ inner tea.Msg }

// chatFailedMsg carries the draft back after a rejected send (e.g. over the length cap), so the
// composer reopens with the text to trim rather than losing it.
type chatFailedMsg struct {
	err   error
	draft string
}

// resumedMsg fires when a tea.ExecProcess child exits: ExecProcess skips its repaint when already
// in the alt screen, losing the footer, so Update answers with a full tea.ClearScreen.
type resumedMsg struct{}
type openEditMsg struct{ t api.Task } // a pre-edit sync returned — open the edit form from this fresh task

// tickMsg drives polling; polledMsg is a polled state, kept distinct so it doesn't re-arm the SSE waiter.
type tickMsg time.Time
type polledMsg api.BoardState

const refreshInterval = 3 * time.Second

// detailScrollStep is how many lines J/K scroll the detail pane at once.
const detailScrollStep = 5
