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
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

// stateMsg is a board snapshot from /events; gen lets a stream abandoned by a repo switch be ignored.
type stateMsg struct {
	st  hub.BoardState
	gen int
}
type logMsg struct {
	key string
	evs []store.Event
}
type prMsg struct {
	key string
	d   hub.PRDetail
}
type taskMsg struct {
	key string
	t   store.Task
}
type paneMsg struct {
	agent string
	text  string
}
type agentPodMsg struct {
	agent string
	text  string
}
type clientsMsg struct {
	agent   string
	clients []hub.ClientView
}
type prLintMsg struct {
	pr   string
	text string
}
type reviewPromptMsg string
type reviewReadyMsg string // the review-workspace path to open a shell in
type editorReadyMsg string // the review-workspace path to open the user's editor on

// openPlanFormMsg routes the picked planner through Update: a choice's apply returns a cmd, but the
// form is model state.
type openPlanFormMsg string

// approveMergeMsg routes the intent through Update so the "merging" marker renders before the async work.
type approveMergeMsg struct{ id string }

// mergeDoneMsg reports a finished merge; either way the transient "merging" marker is cleared.
type mergeDoneMsg struct {
	id    string
	state hub.BoardState
	err   error
}

// taskOpMsg routes a close/scrap through Update — a choice's apply can't mutate the returned
// model — so the transient verb renders before run fires.
type taskOpMsg struct {
	id   string
	verb string
	run  tea.Cmd
}

// taskOpDoneMsg reports a finished close/scrap; either way the transient verb is cleared.
type taskOpDoneMsg struct {
	id    string
	state hub.BoardState
	err   error
}

// paneLines is how many rows of an agent's tmux scrollback the detail shows.
const paneLines = 200

type errMsg struct { // fatal: hub connection lost (unless a stale generation)
	err error
	gen int
}
type errModalMsg struct{ err error } // non-fatal: show the error modal
type chatSentMsg struct{}            // a chat compose sent OK — clear + close the composer

// resumedMsg fires when a tea.ExecProcess child exits: ExecProcess skips its repaint when already
// in the alt screen, losing the footer, so Update answers with a full tea.ClearScreen.
type resumedMsg struct{}
type openEditMsg struct{ t store.Task } // a pre-edit sync returned — open the edit form from this fresh task

// tickMsg drives polling; polledMsg is a polled state, kept distinct so it doesn't re-arm the SSE waiter.
type tickMsg time.Time
type polledMsg hub.BoardState

const refreshInterval = 3 * time.Second

// detailScrollStep is how many lines J/K scroll the detail pane at once.
const detailScrollStep = 5
