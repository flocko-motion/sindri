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

// ClientView is one dial-in on an agent's tmux session; orphaned attaches show up here too. It
// crosses the wire, so it is internal/api.ClientView under the name every existing caller uses.
type ClientView = api.ClientView

// runtimeTTL: a stale runtime label costs nothing, a capture-pane spawn per board read does
// (they don't parallelise — see container.ListByLabelCached).
const runtimeTTL = 2 * time.Second

// Observation is one look at an agent's pane: what the text says, and a digest of the whole screen.
// Both come from ONE capture — taking them separately would double the exec cost of every sweep.
type Observation struct {
	Runtime string // "working" | "blocked" | "idle" | "signed-out" | "" when the capture failed
	Digest  string // "" when the capture failed, so a lost probe never reads as "nothing changed"
	// ToolRunning is whether the pane itself shows a tool call still in flight — a shell that has not
	// returned prints nothing, so Digest alone cannot tell this apart from a frozen turn (-> watchdog.record).
	ToolRunning bool
}

// runtimeMemo memoises Observe's reading per agent key, TTL-bound. A Service field, not a package
// var: a fresh Service (every test constructs its own) then starts with no reading left over from
// another's.
type runtimeMemo struct {
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
	s.runtimeMemo.mu.Lock()
	if at, ok := s.runtimeMemo.at[key]; ok && time.Since(at) < runtimeTTL {
		v := s.runtimeMemo.val[key]
		s.runtimeMemo.mu.Unlock()
		return v
	}
	s.runtimeMemo.mu.Unlock()

	var obs Observation
	out, err := container.ExecContext(ctx, s.deps.ContainerName(project, name), append([]string{"tmux"}, tmux.CapturePane(name, 0, false)...)...) // plain: the text is pattern-matched
	if err == nil {
		obs.Runtime = agentport.Runtime(string(out)) // shared classifier: board + herdr agree
		obs.Digest = fmt.Sprintf("%x", sha256.Sum256(out))
		obs.ToolRunning = agentport.ToolRunning(string(out))
	}
	s.runtimeMemo.mu.Lock()
	if s.runtimeMemo.at == nil {
		s.runtimeMemo.at, s.runtimeMemo.val = map[string]time.Time{}, map[string]Observation{}
	}
	s.runtimeMemo.at[key], s.runtimeMemo.val[key] = time.Now(), obs
	s.runtimeMemo.mu.Unlock()
	return obs
}

// contextTTL: the transcript grows with every turn, not every board read (same reasoning as
// runtimeTTL, longer window since a session file changes far less often than the tmux pane).
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
// pane — that's pattern-matched text). ok=false when no session has recorded usage yet.
func (s *Service) ContextUsage(project, name string) (tokens, window int, model string, ok bool) {
	key := project + "/" + name
	contextMemo.mu.Lock()
	if at, cached := contextMemo.at[key]; cached && time.Since(at) < contextTTL {
		v := contextMemo.val[key]
		contextMemo.mu.Unlock()
		return v.tokens, v.window, v.model, v.ok
	}
	contextMemo.mu.Unlock()
	return s.SampleContext(project, name)
}

// SampleContext reads the transcript itself and leaves the reading where ContextUsage will serve
// it — for the observer's own cadence (-> hub/watchdog.go), sparing every other reader the parse.
func (s *Service) SampleContext(project, name string) (tokens, window int, model string, ok bool) {
	t, w, m, found := agentport.ContextUsage(paths.AgentHomeDir(project, name))
	key := project + "/" + name
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

// ModelMatches reports whether detected is want, via the wired backend — not always a bare
// equality (-> agentport.Agent.ModelMatches).
func (s *Service) ModelMatches(want, detected string) bool {
	return agentport.ModelMatches(want, detected)
}

// ModelInUse picks between the two readings of what an agent runs: the one detected off its
// transcript while it is up — a human may change the model by hand, which the transcript sees first
// — and the recorded choice otherwise, all there is for an agent that is not running.
//
// A function, not a probe: the board already holds both readings from the watchdog's own sample
// (-> hub/watchdog.go), which taking them here again would cost per render, per connected client.
func ModelInUse(recorded, detected string, up bool) string {
	if up && detected != "" {
		return detected
	}
	return recorded
}

// CurrentModel is the model name is effectively running, taking both readings itself. For a caller
// with neither — the assignment path; the board must not use it, since the probe is per agent.
func (s *Service) CurrentModel(ctx context.Context, project, name string) string {
	up := s.AgentAlive(ctx, project, name)
	var detected string
	if up {
		_, _, detected, _ = s.ContextUsage(project, name)
	}
	return ModelInUse(s.recordedModel(project, name), detected, up)
}

// recordedModel is the model the roster says an agent was started on, "" if it cannot be read.
func (s *Service) recordedModel(project, name string) string {
	a, ok, err := s.store.For(project).GetAgent(name)
	if err != nil || !ok {
		return ""
	}
	return a.Model
}

// ForgetContext drops every standing reading of name's context — this package's memo and the hub's
// own sample. For the one caller that KNOWS the previous measurement is now wrong because it just
// invalidated it: clearing or compacting a session (-> ClearContext, Compact).
//
// Both stores, not a shorter TTL: the reading survived the very act that made it false once before,
// telling a just-cleared agent it was still full. The board reads the sample, the gate reads the
// memo — leaving either behind puts that bug back on the half left standing.
func (s *Service) ForgetContext(project, name string) {
	key := project + "/" + name
	contextMemo.mu.Lock()
	delete(contextMemo.at, key)
	delete(contextMemo.val, key)
	contextMemo.mu.Unlock()
	s.deps.ForgetFill(project, name)
}

// LaunchDiagnostic re-runs both liveness probes so a launch timeout says which one failed.
func (s *Service) LaunchDiagnostic(ctx context.Context, project, name string) string {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	c := s.deps.ContainerName(project, name)
	if !container.RunningContext(ctx, c) {
		return fmt.Sprintf("the runtime does not report container %s as running [%s]", c,
			container.Diagnose(ctx, c))
	}
	if out, err := container.ExecContext(ctx, c, append([]string{"tmux"}, tmux.HasSession(name)...)...); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Sprintf("container is running but its tmux session check failed: %s", msg)
	}
	return "container and session both answer now — the liveness checks had been failing transiently"
}

// AgentDiagnostic un-collapses the board's "down" into both probes' real results.
func (s *Service) AgentDiagnostic(ctx context.Context, project, name string) string {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	c := s.deps.ContainerName(project, name)
	var b strings.Builder
	fmt.Fprintf(&b, "container:      %s\n", c)
	fmt.Fprintf(&b, "running check:  %s\n", container.Diagnose(ctx, c))

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

// AgentAlive reports whether an agent is running: pod up AND tmux session live, under a probeTimeout
// cut from the caller's ctx — a wedged pod reads "down" rather than blocking whoever asked.
func (s *Service) AgentAlive(ctx context.Context, project, name string) bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	return container.RunningContext(ctx, s.deps.ContainerName(project, name)) && s.SessionAliveCtx(ctx, project, name)
}

// Clients lists an agent's dial-ins; a wedged exec degrades to "not running".
func (s *Service) Clients(ctx context.Context, project, name string) ([]ClientView, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
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

// SessionAliveCtx reports whether the agent's tmux session is up inside its pod, bounded by ctx.
func (s *Service) SessionAliveCtx(ctx context.Context, project, name string) bool {
	_, err := container.ExecContext(ctx, s.deps.ContainerName(project, name), append([]string{"tmux"}, tmux.HasSession(name)...)...)
	return err == nil
}

// paneTTL bounds how stale a shown pane can be. It is the debounce as much as the cache: the board
// re-renders on every cursor move, and each capture is a container exec — 1.3s on a loaded host.
const paneTTL = 2 * time.Second

// paneMemo memoises AgentPane's capture per agent+lines key, TTL-bound. A Service field, not a
// package var, for the same reason runtimeMemo is: no reading survives past its own Service.
type paneMemo struct {
	mu  sync.Mutex
	at  map[string]time.Time
	val map[string]string
}

// AgentPane shows the live tmux screen, else startup logs, else captured launch output. The capture
// is attempted rather than preceded by a liveness check: asking `tmux has-session` first doubled the
// wait to predict what the capture itself reports, and left a window for the session to die between.
func (s *Service) AgentPane(ctx context.Context, project, name string, lines int) (string, error) {
	key := fmt.Sprintf("%s/%s/%d", project, name, lines)
	s.paneMemo.mu.Lock()
	if at, ok := s.paneMemo.at[key]; ok && time.Since(at) < paneTTL {
		v := s.paneMemo.val[key]
		s.paneMemo.mu.Unlock()
		return v, nil
	}
	s.paneMemo.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, err := container.ExecContext(ctx, s.deps.ContainerName(project, name), append([]string{"tmux"}, tmux.CapturePane(name, lines, true)...)...) // colour: the preview renders ANSI
	pane := string(out)
	if err != nil {
		// No session to capture: what a human wants next is why — the pod's own output, then whatever
		// the launch printed. Not cached, since it is the failing path and its answer changes.
		if logs := container.Logs(s.deps.ContainerName(project, name), lines); logs != "" {
			return logs, nil
		}
		return s.LaunchOutput(project, name), nil
	}
	s.paneMemo.mu.Lock()
	if s.paneMemo.at == nil {
		s.paneMemo.at, s.paneMemo.val = map[string]time.Time{}, map[string]string{}
	}
	s.paneMemo.at[key], s.paneMemo.val[key] = time.Now(), pane
	s.paneMemo.mu.Unlock()
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
