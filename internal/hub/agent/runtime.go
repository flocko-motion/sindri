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
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/tools/paths"
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

// Observation is one look at an agent's pane: what the text says, and a digest of the whole screen.
// Both come from ONE capture — the digest is what tells activity from stillness, and taking it
// separately would double the exec cost of every sweep.
type Observation struct {
	Runtime string // "working" | "blocked" | "idle" | "signed-out" | "" when the capture failed
	Digest  string // "" when the capture failed, so a lost probe never reads as "nothing changed"
}

var runtimeMemo struct {
	mu  sync.Mutex
	at  map[string]time.Time
	val map[string]Observation
}

// RuntimeState classifies Claude's pane: "working", "blocked", "idle", or "" when unknown.
func (s *Service) RuntimeState(ctx context.Context, project, name string) string {
	return s.Observe(ctx, project, name).Runtime
}

// Observe captures an agent's pane once and reports both what it says and what it looks like.
func (s *Service) Observe(ctx context.Context, project, name string) Observation {
	key := project + "/" + name
	runtimeMemo.mu.Lock()
	if at, ok := runtimeMemo.at[key]; ok && time.Since(at) < runtimeTTL {
		v := runtimeMemo.val[key]
		runtimeMemo.mu.Unlock()
		return v
	}
	runtimeMemo.mu.Unlock()

	var obs Observation
	out, err := container.ExecContext(ctx, s.deps.ContainerName(project, name), append([]string{"tmux"}, tmux.CapturePane(name, 0, false)...)...) // plain: the text is pattern-matched
	if err == nil {
		obs.Runtime = agentport.Runtime(string(out)) // shared classifier: board + herdr agree
		obs.Digest = fmt.Sprintf("%x", sha256.Sum256(out))
	}
	runtimeMemo.mu.Lock()
	if runtimeMemo.at == nil {
		runtimeMemo.at, runtimeMemo.val = map[string]time.Time{}, map[string]Observation{}
	}
	runtimeMemo.at[key], runtimeMemo.val[key] = time.Now(), obs
	runtimeMemo.mu.Unlock()
	return obs
}

// contextTTL: the transcript grows with every turn, not every board read, so a read straight off
// disk per request buys nothing over a short memo (same reasoning as runtimeTTL, longer window
// because a session file changes far less often than the tmux pane does).
const contextTTL = 15 * time.Second

// contextSample is one reading: what the session carries, the window it has to fill, and the model
// carrying it.
type contextSample struct {
	tokens, window int
	model          string
	ok             bool
}

var contextMemo struct {
	mu  sync.Mutex
	at  map[string]time.Time
	val map[string]contextSample
}

// ContextUsage reads name's live session context size, window and model off disk (never the tmux
// pane — that's pattern-matched text, this is exact usage from the transcript itself). ok=false when
// no session has recorded usage yet.
func (s *Service) ContextUsage(project, name string) (tokens, window int, model string, ok bool) {
	key := project + "/" + name
	contextMemo.mu.Lock()
	if at, cached := contextMemo.at[key]; cached && time.Since(at) < contextTTL {
		v := contextMemo.val[key]
		contextMemo.mu.Unlock()
		return v.tokens, v.window, v.model, v.ok
	}
	contextMemo.mu.Unlock()

	t, w, m, found := agentport.ContextUsage(paths.AgentHomeDir(project, name))
	contextMemo.mu.Lock()
	if contextMemo.at == nil {
		contextMemo.at, contextMemo.val = map[string]time.Time{}, map[string]contextSample{}
	}
	contextMemo.at[key], contextMemo.val[key] = time.Now(), contextSample{t, w, m, found}
	contextMemo.mu.Unlock()
	return t, w, m, found
}

// CompactionThreshold reports the token count above which a session filling window tokens is worth
// compacting, from the wired backend's own formula for the model that window belongs to.
func (s *Service) CompactionThreshold(window int) int { return agentport.CompactionThreshold(window) }

// ModelWindow resolves model to its context window via the wired backend, ok=false when it is not
// recognised — the check a chosen model must pass before an agent is started on it.
func (s *Service) ModelWindow(model string) (int, bool) { return agentport.ModelWindow(model) }

// ModelForTier resolves tier to the model it dispatches to, via the wired backend.
func (s *Service) ModelForTier(tier string) (string, bool) { return agentport.ModelForTier(tier) }

// CurrentModel is the model name is effectively running: detected off its transcript while alive
// (a human may change it by hand inside Claude Code, which the transcript sees first), the stored
// choice otherwise — all there is for one that is not running.
func (s *Service) CurrentModel(project, name string) string {
	if s.AgentAlive(project, name) {
		if _, _, detected, ok := s.ContextUsage(project, name); ok && detected != "" {
			return detected
		}
	}
	a, ok, err := s.store.For(project).GetAgent(name)
	if err != nil || !ok {
		return ""
	}
	return a.Model
}

// ForgetContext drops name's memoised context reading. For the one caller that KNOWS the previous
// measurement is now wrong because it just invalidated it: clearing a session (-> ClearContext).
//
// Here rather than in a shorter TTL. The memo exists so the frequent idle poll does not re-read a
// transcript per request, and that is still right for every other reader — but the reading survived
// the very act that made it false, so the hub answered the kickoff after a clear from the pre-clear
// figure and told the agent it was still full.
func (s *Service) ForgetContext(project, name string) {
	key := project + "/" + name
	contextMemo.mu.Lock()
	delete(contextMemo.at, key)
	delete(contextMemo.val, key)
	contextMemo.mu.Unlock()
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
	// Both probes pass on an agent that cannot run a turn — everything up, nothing moving, which is
	// the case a diagnostic gets asked about.
	if s.RuntimeState(ctx, project, name) == string(agentport.SignedOut) {
		fmt.Fprintf(&b, "runtime:        SIGNED OUT — the pane says to run /login, so it cannot run a turn "+
			"and nothing typed into it is sent. The hub keeps the host's credentials staged in its home; "+
			"`sindri agent restart %s` makes the process re-read them (the session resumes). If the host is "+
			"signed out too, log in there first.\n", name)
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

// SessionAlive reports whether the agent's tmux session is up inside its pod.
func (s *Service) SessionAlive(project, name string) bool {
	return s.SessionAliveCtx(context.Background(), project, name)
}

// SessionAliveCtx is SessionAlive bounded by ctx.
func (s *Service) SessionAliveCtx(ctx context.Context, project, name string) bool {
	_, err := container.ExecContext(ctx, s.deps.ContainerName(project, name), append([]string{"tmux"}, tmux.HasSession(name)...)...)
	return err == nil
}

// paneTTL bounds how stale a shown pane can be. It is the debounce as much as the cache: the board
// re-renders on every cursor move, and each capture is a container exec — 1.3s on a loaded host.
const paneTTL = 2 * time.Second

var paneMemo struct {
	mu  sync.Mutex
	at  map[string]time.Time
	val map[string]string
}

// AgentPane shows the live tmux screen, else startup logs, else captured launch output.
//
// The capture is attempted rather than preceded by a liveness check: asking `tmux has-session` first
// spent a whole exec — doubling the wait before anything appeared — to predict what the capture
// itself reports, and left a window for the session to die between the two answers.
func (s *Service) AgentPane(project, name string, lines int) (string, error) {
	key := fmt.Sprintf("%s/%s/%d", project, name, lines)
	paneMemo.mu.Lock()
	if at, ok := paneMemo.at[key]; ok && time.Since(at) < paneTTL {
		v := paneMemo.val[key]
		paneMemo.mu.Unlock()
		return v, nil
	}
	paneMemo.mu.Unlock()

	out, err := container.Exec(s.deps.ContainerName(project, name), append([]string{"tmux"}, tmux.CapturePane(name, lines, true)...)...) // colour: the preview renders ANSI
	pane := string(out)
	if err != nil {
		// No session to capture: what a human wants next is why — the pod's own output, then whatever
		// the launch printed. Not cached, since it is the failing path and its answer changes.
		if logs := container.Logs(s.deps.ContainerName(project, name), lines); logs != "" {
			return logs, nil
		}
		return s.LaunchOutput(project, name), nil
	}
	paneMemo.mu.Lock()
	if paneMemo.at == nil {
		paneMemo.at, paneMemo.val = map[string]time.Time{}, map[string]string{}
	}
	paneMemo.at[key], paneMemo.val[key] = time.Now(), pane
	paneMemo.mu.Unlock()
	return pane, nil
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
