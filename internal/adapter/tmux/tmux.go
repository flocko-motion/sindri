// package: adapter/tmux / tmux
// type:    adapter (external tool: tmux)
// job:     construct tmux command argv — new-session, send-keys (the inbound
// injection primitive), attach, capture-pane. Pure argv builders:
// tmux runs *inside* an agent's pod, so execution is the pod adapter's
// job (host composes pod.Exec(pod, tmux.X(...)...)).
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

// Interrupt builds a bare Escape keypress — "abort the current operation" to Claude and most TUIs.
// A key NAME, not -l literal, or tmux would deliver the letters "E","s","c".
func Interrupt(session string) []string {
	return []string{"send-keys", "-t", session, "Escape"}
}

// Attach builds the human dial-in; readOnly observes with -r. A read-write attach adds -d to evict
// other clients, because a dropped `podman exec` leaves one wedged on a dead pty, and sharing the
// session with it is what made a fresh attach "see it but can't type". Observers skip -d.
func Attach(session string, readOnly bool) []string {
	args := []string{"attach-session", "-t", session}
	if readOnly {
		return append(args, "-r")
	}
	return append(args, "-d")
}

// ListClients lists each attached client as "tty width height readonly", space-separated since a
// tty never contains spaces. Errors when the session is absent, so it doubles as a liveness probe.
func ListClients(session string) []string {
	return []string{"list-clients", "-t", session, "-F", "#{client_tty} #{client_width} #{client_height} #{client_readonly}"}
}

// HasSession builds `tmux has-session -t <session>` — exits 0 iff the session
// exists. The true "is the agent alive / attachable" probe: the pod itself is
// just a sleep that outlives Claude, so the container running ≠ agent alive.
func HasSession(session string) []string {
	return []string{"has-session", "-t", session}
}

// CapturePane dumps a session's pane; lines>0 reaches that far back into the scrollback. color
// keeps the escape sequences for the TUI preview — leave it off when parsing, or they corrupt it.
func CapturePane(session string, lines int, color bool) []string {
	args := []string{"capture-pane", "-t", session, "-p"}
	if color {
		args = append(args, "-e")
	}
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}
	return args
}
