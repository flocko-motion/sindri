// package: ui/attach / herdr
// type:    application helper (composes adapters for the UIs)
// job:     the single place that lists an agent in herdr's sidebar for the lifetime
//          of an interactive attach — used by every attach path (`agent attach`,
//          `coauthor`, the TUI): reports the agent by name with a live state,
//          refreshing until detach so it never goes stale, then releases the pane.
// limits:  best-effort UI nicety — a no-op outside a herdr pane, and pane-probe
//          failures keep the last state rather than disturbing the attach. Client
//          side: it captures the pane directly (like `agent attach` does), not over
//          the hub socket.
package attach

import (
	"context"
	"sync"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/herdr"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/container"
)

// refreshInterval is how often the live state is re-reported to herdr during an
// attach. Frequent enough that a state change (blocked↔working) shows up promptly,
// cheap enough that the per-tick pane capture is unnoticeable.
const refreshInterval = 3 * time.Second

// ReportToHerdr lists the agent (cname is its container, name its tmux session) in
// herdr's sidebar for the duration of an attach: it reports the initial state, then
// re-reports every few seconds by classifying the live pane, until the returned stop
// is called — which releases herdr's authority so the pane falls back to herdr's own
// detection. A no-op outside a herdr pane (stop is then a no-op too). Every attach
// path calls this, so all agents report identically — worker, coauthor, TUI or CLI.
func ReportToHerdr(cname, name string) (stop func()) {
	if !herdr.InPane() {
		return func() {}
	}
	herdr.Report(name, herdrState(cname, name))
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(refreshInterval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				herdr.ReportState(name, herdrState(cname, name))
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			herdr.Release()
		})
	}
}

// herdrState captures the agent's pane and classifies it into herdr's state
// vocabulary. A probe failure maps to herdr's default (working) rather than a wrong
// "blocked"/"idle": best-effort, and the next tick corrects it.
func herdrState(cname, name string) string {
	ctx, cancel := context.WithTimeout(context.Background(), refreshInterval)
	defer cancel()
	out, err := container.ExecContext(ctx, cname, append([]string{"tmux"}, tmux.CapturePane(name, 0, false)...)...)
	if err != nil {
		return herdr.State("") // unknown → working
	}
	return herdr.State(agentport.Runtime(string(out)))
}
