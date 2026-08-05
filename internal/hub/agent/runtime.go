// package: hub/agent / runtime
// type:    logic (agent runtime inspection)
// job:     read what a running agent is doing — liveness (pod up + tmux session),
// Claude's live runtime state, the attached dial-in clients, the visible
// pane, pod info, and human-readable liveness diagnostics. The board and the
// UIs render these; the workflow uses the liveness checks.
// limits:  read-only probes over the container runtime + tmux; the tmux session is
// named after the agent. No lifecycle changes here.
package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/container"
)

// probeTimeout bounds each probe: a container that can't answer reads "down", it doesn't stall.
const probeTimeout = 3 * time.Second

// ClientView is one dial-in on an agent's tmux session; orphaned attaches show up here too.
// It crosses the wire, so it is internal/api.ClientView under the name every existing
// caller here already uses.
type ClientView = api.ClientView

// runtimeTTL: a stale runtime label costs nothing, a capture-pane spawn per board read does
// (they don't parallelise — see container.ListByLabelCached).
const runtimeTTL = 2 * time.Second

var runtimeMemo struct {
	mu  sync.Mutex
	at  map[string]time.Time
	val map[string]string
}

// RuntimeState classifies Claude's pane: "working", "blocked", "idle", or "" when unknown.
func (s *Service) RuntimeState(ctx context.Context, project, name string) string {
	key := project + "/" + name
	runtimeMemo.mu.Lock()
	if at, ok := runtimeMemo.at[key]; ok && time.Since(at) < runtimeTTL {
		v := runtimeMemo.val[key]
		runtimeMemo.mu.Unlock()
		return v
	}
	runtimeMemo.mu.Unlock()

	var state string
	out, err := container.ExecContext(ctx, s.deps.ContainerName(project, name), append([]string{"tmux"}, tmux.CapturePane(name, 0, false)...)...) // plain: the text is pattern-matched
	if err == nil {
		state = agentport.Runtime(string(out)) // shared classifier: board + herdr agree
	}
	runtimeMemo.mu.Lock()
	if runtimeMemo.at == nil {
		runtimeMemo.at, runtimeMemo.val = map[string]time.Time{}, map[string]string{}
	}
	runtimeMemo.at[key], runtimeMemo.val[key] = time.Now(), state
	runtimeMemo.mu.Unlock()
	return state
}

// LaunchDiagnostic re-runs both liveness probes so a launch timeout says which one failed.
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

// AgentDiagnostic un-collapses the board's "down" into both probes' real results.
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

// AgentAliveCtx is AgentAlive bounded by ctx, so a wedged pod reads "down" instead of blocking.
func (s *Service) AgentAliveCtx(ctx context.Context, project, name string) bool {
	return container.RunningContext(ctx, s.deps.ContainerName(project, name)) && s.SessionAliveCtx(ctx, project, name)
}

// Clients lists an agent's dial-ins; a wedged exec degrades to "not running".
func (s *Service) Clients(project, name string) ([]ClientView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cs, ok := s.ClientsCtx(ctx, project, name)
	if !ok {
		return nil, fmt.Errorf("agent %q is not running", name)
	}
	return cs, nil
}

// ClientsCtx parses `tmux list-clients`; ok=false means no session, so it doubles as a probe.
func (s *Service) ClientsCtx(ctx context.Context, project, name string) (cs []ClientView, ok bool) {
	out, err := container.ExecContext(ctx, s.deps.ContainerName(project, name), append([]string{"tmux"}, tmux.ListClients(name)...)...)
	if err != nil {
		return nil, false
	}
	return parseClients(string(out)), true
}

// parseClients reads "tty width height readonly" lines; malformed ones are skipped.
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

// FormatClients is shared by CLI `agent info` and the TUI detail view, so both read alike.
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

// AgentPane shows the live tmux screen, else startup logs, else captured launch output.
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
