// package: hub/agent / runtime
// type:    logic (agent runtime inspection)
// job:     read what a running agent is doing — liveness (pod up + tmux session),
//          Claude's live runtime state, the attached dial-in clients, the visible
//          pane, pod info, and human-readable liveness diagnostics. The board and the
//          UIs render these; the workflow uses the liveness checks.
// limits:  read-only probes over the container runtime + tmux; the tmux session is
//          named after the agent. No lifecycle changes here.
package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/container"
)

// probeTimeout bounds each runtime probe. A container that can't answer within this
// window is reported "down" rather than stalling the read.
const probeTimeout = 3 * time.Second

// ClientView is one human attached to an agent's tmux session — a live dial-in.
// Surfaced so the UIs can show who's watching and whether they can type (a read-only
// client observes but can't send keys). An orphaned client (a dropped exec that left
// its tmux attach behind) shows up here too.
type ClientView struct {
	TTY      string `json:"tty"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	ReadOnly bool   `json:"read_only"`
}

// RuntimeState captures an agent's Claude pane and classifies what Claude is doing
// right now — "working", "blocked" (waiting on input), "idle" — or "" when it can't
// tell. Bounded by ctx so a wedged capture can't stall the read.
func (s *Service) RuntimeState(ctx context.Context, project, name string) string {
	out, err := container.ExecContext(ctx, s.deps.ContainerName(project, name), append([]string{"tmux"}, tmux.CapturePane(name, 0, false)...)...) // plain: the text is pattern-matched
	if err != nil {
		return ""
	}
	switch agentport.DetectState(string(out)) {
	case agentport.Working:
		return "working"
	case agentport.Blocked:
		return "blocked"
	case agentport.Idle:
		return "idle"
	}
	return "idle" // a shell (maintenance mode) or unrecognized screen = not doing anything
}

// LaunchDiagnostic reports WHY a just-launched agent isn't observed up, so a timeout
// is actionable: it re-runs the two liveness probes, capturing whichever fails.
func (s *Service) LaunchDiagnostic(project, name string) string {
	c := s.deps.ContainerName(project, name)
	if !container.Running(c) {
		return fmt.Sprintf("the runtime does not report container %s as running [%s]", c,
			container.Diagnose(context.Background(), c))
	}
	if out, err := container.Exec(c, append([]string{"tmux"}, tmux.HasSession(name)...)...); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Sprintf("container is running but its tmux session check failed: %s", msg)
	}
	return "container and session both answer now — the liveness checks had been failing transiently"
}

// AgentDiagnostic reports, as one human string, what BOTH liveness probes observe —
// the running check and the tmux session check — each with its real result rather
// than the single "down" the board collapses them into. The session exec is
// time-bounded, so a WEDGED exec is reported as a timeout, not a hang.
func (s *Service) AgentDiagnostic(project, name string) string {
	c := s.deps.ContainerName(project, name)
	var b strings.Builder
	fmt.Fprintf(&b, "container:      %s\n", c)
	fmt.Fprintf(&b, "running check:  %s\n", container.Diagnose(context.Background(), c))

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	out, err := container.ExecContext(ctx, c, append([]string{"tmux"}, tmux.HasSession(name)...)...)
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		fmt.Fprintf(&b, "session check:  TIMED OUT after %s — `tmux has-session` in the container did not return (exec is wedged); this is why liveness reads 'down'\n", probeTimeout)
	case err != nil:
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		fmt.Fprintf(&b, "session check:  FAILED: %s\n", msg)
	default:
		fmt.Fprintf(&b, "session check:  ok — tmux session %q answers\n", name)
	}
	return b.String()
}

// AgentAlive reports whether an agent is running (pod up and tmux session live).
func (s *Service) AgentAlive(project, name string) bool {
	return s.AgentAliveCtx(context.Background(), project, name)
}

// AgentAliveCtx is AgentAlive with each probe bounded by ctx, so a wedged pod times
// out to "down" instead of blocking. Used by the board read.
func (s *Service) AgentAliveCtx(ctx context.Context, project, name string) bool {
	return container.RunningContext(ctx, s.deps.ContainerName(project, name)) && s.SessionAliveCtx(ctx, project, name)
}

// Clients lists the humans attached to an agent's tmux session (dial-ins). Errors
// when the agent isn't running. Bounded: a wedged exec degrades to "not running".
func (s *Service) Clients(project, name string) ([]ClientView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cs, ok := s.ClientsCtx(ctx, project, name)
	if !ok {
		return nil, fmt.Errorf("agent %q is not running", name)
	}
	return cs, nil
}

// ClientsCtx parses `tmux list-clients` for the agent's session, bounded by ctx.
// ok=false when the session is absent (so it also serves as a liveness probe).
func (s *Service) ClientsCtx(ctx context.Context, project, name string) (cs []ClientView, ok bool) {
	out, err := container.ExecContext(ctx, s.deps.ContainerName(project, name), append([]string{"tmux"}, tmux.ListClients(name)...)...)
	if err != nil {
		return nil, false
	}
	return parseClients(string(out)), true
}

// parseClients turns list-clients output (one "tty width height readonly" line per
// client) into ClientViews. Malformed lines are skipped.
func parseClients(out string) []ClientView {
	var cs []ClientView
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		w, _ := strconv.Atoi(f[1])
		ht, _ := strconv.Atoi(f[2])
		cs = append(cs, ClientView{TTY: f[0], Width: w, Height: ht, ReadOnly: f[3] == "1"})
	}
	return cs
}

// FormatClients renders attached clients for a human — shared by the CLI's
// `agent info` and the TUI detail view so both read identically.
func FormatClients(cs []ClientView) string {
	if len(cs) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "clients:   %d attached\n", len(cs))
	for _, c := range cs {
		mode := "read-write"
		if c.ReadOnly {
			mode = "read-only"
		}
		fmt.Fprintf(&b, "  %s  %dx%d  %s\n", c.TTY, c.Width, c.Height, mode)
	}
	return b.String()
}

// SessionAlive reports whether the agent's tmux session is up inside its pod.
func (s *Service) SessionAlive(project, name string) bool {
	return s.SessionAliveCtx(context.Background(), project, name)
}

// SessionAliveCtx is SessionAlive bounded by ctx.
func (s *Service) SessionAliveCtx(ctx context.Context, project, name string) bool {
	_, err := container.ExecContext(ctx, s.deps.ContainerName(project, name), append([]string{"tmux"}, tmux.HasSession(name)...)...)
	return err == nil
}

// AgentPane returns the last `lines` rows of what the agent is showing — the live
// tmux screen once up, else the container's startup logs, else the captured launch
// output. Empty when truly down.
func (s *Service) AgentPane(project, name string, lines int) (string, error) {
	if s.SessionAlive(project, name) {
		out, err := container.Exec(s.deps.ContainerName(project, name), append([]string{"tmux"}, tmux.CapturePane(name, lines, true)...)...) // colour: the preview renders ANSI
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	if logs := container.Logs(s.deps.ContainerName(project, name), lines); logs != "" {
		return logs, nil
	}
	return s.LaunchOutput(project, name), nil
}

// PodInfo returns a short summary of an agent's container for the Agents-tab pod view.
func (s *Service) PodInfo(project, name string) (string, error) {
	c := s.deps.ContainerName(project, name)
	header := fmt.Sprintf("engine:    %s\ncontainer: %s\n\n", container.Name(), c)
	if info := container.Info(c); info != "" {
		return header + info, nil
	}
	return header + "(no container — agent is down)", nil
}
