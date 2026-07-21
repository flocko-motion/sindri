// package: tui / messages
// type:    ui (Bubble Tea message types)
// job:     the messages the update loop reacts to — board snapshots, lazily
//          fetched detail (log/pr/task/pane/pod), and the poll/error signals —
//          plus the few timing constants that govern live updates.
// limits:  message type definitions only; the loop that reacts to them is in
//          tui.go (-> Update).
package tui

import (
	"time"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

// stateMsg carries a board snapshot from the /events waiter, tagged with the
// subscription generation it came from — so snapshots (and closes) from a stream
// abandoned by a repo switch can be ignored.
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

// approveMergeMsg carries the approve-then-merge intent from the "approve & merge"
// choice back into Update, so the transient "merging" marker is set on the model
// (and rendered) before the async approve+merge runs.
type approveMergeMsg struct{ id string }

// mergeDoneMsg reports a merge attempt finished: on success it carries a fresh
// board snapshot; on failure, the error. Either way the row's transient "merging"
// marker is cleared (replaced by the real status, or reverted on error).
type mergeDoneMsg struct {
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

// resumedMsg fires when an interactive child process launched via tea.ExecProcess
// (a tmux attach, a workspace shell) exits and the TUI resumes. Bubble Tea's
// ExecProcess restore path skips its alt-screen repaint when it was already in the
// alt screen (the renderer's altScreen flag survives the release), so without a
// nudge the resumed frame is never redrawn and the bottom row (the footer) is lost.
// Update answers this with a full tea.ClearScreen — the same remedy the resize path
// uses — forcing a clean repaint.
type resumedMsg struct{}
type openEditMsg struct{ t store.Task } // a pre-edit sync returned — open the edit form from this fresh task

// tickMsg drives periodic polling; polledMsg carries a state fetched by a poll
// (distinct from stateMsg so it doesn't re-arm the SSE waiter).
type tickMsg time.Time
type polledMsg hub.BoardState

const refreshInterval = 3 * time.Second

// detailScrollStep is how many lines J/K scroll the detail pane at once.
const detailScrollStep = 5
