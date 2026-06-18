// package: adapter/tmux / tmux
// type:    adapter (external tool: tmux)
// job:     construct tmux command argv — new-session, send-keys (the inbound
//          injection primitive), attach, capture-pane. Pure argv builders:
//          tmux runs *inside* an agent's pod, so execution is the pod adapter's
//          job (host composes pod.Exec(pod, tmux.X(...)...)).
// limits:  no execution here; knows nothing of pods, agents, or the hub.
package tmux

import "fmt"

// SendText builds the argv pair that injects text "as if typed": the literal
// text (—l -- so brackets/spaces/provenance tags are never interpreted as tmux
// key names), then a separate Enter to submit. Returns one argv per command.
func SendText(session, text string) [][]string {
	return [][]string{
		{"send-keys", "-t", session, "-l", "--", text},
		{"send-keys", "-t", session, "Enter"},
	}
}

// Attach builds `tmux attach-session -t <session>` (+ -r for read-only) — the
// human dial-in.
func Attach(session string, readOnly bool) []string {
	args := []string{"attach-session", "-t", session}
	if readOnly {
		args = append(args, "-r")
	}
	return args
}

// HasSession builds `tmux has-session -t <session>` — exits 0 iff the session
// exists. The true "is the agent alive / attachable" probe: the pod itself is
// just a sleep that outlives Claude, so the container running ≠ agent alive.
func HasSession(session string) []string {
	return []string{"has-session", "-t", session}
}

// CapturePane builds `tmux capture-pane -p` to dump a session's pane as plain
// text — a read-only peek at what the agent is showing. lines>0 reaches that
// many rows back into the scrollback (-S -<lines>); 0 captures the visible
// screen only.
func CapturePane(session string, lines int) []string {
	args := []string{"capture-pane", "-t", session, "-p"}
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}
	return args
}
