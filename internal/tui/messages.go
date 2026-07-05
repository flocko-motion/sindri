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
type prLintMsg struct {
	pr   string
	text string
}
type reviewPromptMsg string
type reviewReadyMsg string // the review-workspace path to open a shell in

// paneLines is how many rows of an agent's tmux scrollback the detail shows.
const paneLines = 200

type errMsg struct {                 // fatal: hub connection lost (unless a stale generation)
	err error
	gen int
}
type errModalMsg struct{ err error } // non-fatal: show the error modal

// tickMsg drives periodic polling; polledMsg carries a state fetched by a poll
// (distinct from stateMsg so it doesn't re-arm the SSE waiter).
type tickMsg time.Time
type polledMsg hub.BoardState

const refreshInterval = 3 * time.Second

// detailScrollStep is how many lines J/K scroll the detail pane at once.
const detailScrollStep = 5
