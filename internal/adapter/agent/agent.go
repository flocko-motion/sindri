// package: adapter/agent / agent
// type:    logic (the coding-agent PORT — hexagonal abstraction)
// job:     the contract sindri needs from a coding agent (Claude Code today): classify a pane's
// runtime state, and provision an agent's home from a composed prompt. A backend implements it.
// limits:  no agent specifics here (-> adapter/agent/claude, which depends on this
// package, never the reverse).
package agent

import (
	"io"
	"math"
)

// State is what the agent tool is doing now, not sindri's workflow phase.
type State string

const (
	Working State = "working" // actively processing a turn
	Blocked State = "blocked" // waiting for a user response
	Idle    State = "idle"    // stopped at the prompt, nothing happening
	// SignedOut: the tool has no valid credentials, so it cannot run a turn at all. Distinct from
	// Blocked, which a message answers — nothing typed at a signed-out prompt is ever sent.
	SignedOut State = "signed-out"
	// Failed: the turn was cut off by the API, so nothing is running however the pane looks. It says
	// so in words and nothing else can tell — the interrupt hint and the spinner both survive it.
	Failed  State = "api-error"
	Unknown State = "unknown" // not classifiable (shell, transcript viewer, boot, …)
)

// HomeSpec is what a backend needs to provision one agent's home. The workflow composes
// the prompt (high-level logic); the backend only writes tool-specific files around it.
type HomeSpec struct {
	Dir          string    // host home dir to create + populate (mounted into the pod)
	SystemPrompt string    // composed by the workflow; the backend persists it verbatim
	Out          io.Writer // setup announcements (e.g. a one-time credential-access prompt)
	// Workspace is the host path of the tree the pod mounts at /workspace. The backend reads it to
	// decide which language tooling is worth declaring — a pod with no Go project should get no Go
	// language server.
	Workspace string
}

// Home is a provisioned home ready to mount (host paths); without HasCreds the caller
// can't run the agent authenticated.
type Home struct {
	Dir        string
	ConfigPath string
	HasCreds   bool
}

// Agent is the port: what the hub needs from a coding-agent backend.
type Agent interface {
	// DetectState classifies pane text as `tmux capture-pane -p` yields it.
	DetectState(screen string) State
	// PrepareHome provisions spec.Dir and returns the host paths to mount.
	PrepareHome(spec HomeSpec) (Home, error)
	// RestageCredentials carries the host's credentials into an already-provisioned home when the
	// host's reach further, so a re-login on the host reaches a pod that is already running.
	// Reports whether it wrote.
	RestageCredentials(dir string) (bool, error)
	// HostTokenExpiry is when the host's access token lapses (epoch ms), and whether there is a
	// usable one. Every agent runs on this single token, so its expiry is the moment the whole fleet
	// needs the replacement — the hub watches it to redistribute then rather than on a slow tick.
	HostTokenExpiry() (expiresAtMS int64, usable bool)
	// ContextUsage reports what the live session under home carries, the window it fills, and the
	// model carrying it, all from the backend's own transcript. The window comes from here because
	// only the backend knows which model answers; a caller that assumed one retired workers with
	// most of 1M unused. ok=false when nothing has been recorded yet.
	ContextUsage(home string) (tokens, window int, model string, ok bool)
	// CompactionThreshold is the token count above which a session filling window tokens is worth
	// compacting — from the same backend ContextUsage's window came from, since only it knows the
	// shape of its own context-management economics.
	CompactionThreshold(window int) int
	// ModelWindow resolves a model id to its context window, ok=false when the backend does not
	// recognise it — the check a chosen model must pass before the hub starts an agent on it.
	ModelWindow(model string) (window int, ok bool)
	// ModelForTier resolves a difficulty tier to the model it dispatches to, ok=false for anything
	// the backend does not recognise — the dispatcher's own mapping, from the backend since a
	// second implementation would have its own names for the same idea.
	ModelForTier(tier string) (model string, ok bool)
	// ToolRunning reports whether the pane shows a tool call in flight (a shell, or anything else the
	// backend renders the same way) — evidence the screen is quiet because nothing has RETURNED yet,
	// not because the turn is stuck. Separate from DetectState: the state stays Working either way,
	// and only the caller measuring stillness needs to know which kind of "unchanged" this is.
	ToolRunning(screen string) bool
}

// active is wired once at startup via Use; the no-op default keeps the port safe before.
var active Agent = noop{}

// Use selects the coding-agent backend for this process. Called once at startup.
func Use(a Agent) { active = a }

// DetectState classifies a pane via the wired backend.
func DetectState(screen string) State { return active.DetectState(screen) }

// Runtime is the single source of the "working"|"blocked"|"idle"|"signed-out" word every reader
// shares. An unrecognized screen counts as idle: nothing needs surfacing.
func Runtime(screen string) string {
	switch s := DetectState(screen); s {
	case Working, Blocked, SignedOut, Failed:
		return string(s)
	default: // Idle or Unknown
		return "idle"
	}
}

// PrepareHome provisions an agent home via the wired backend.
func PrepareHome(spec HomeSpec) (Home, error) { return active.PrepareHome(spec) }

// RestageCredentials refreshes one home's credentials from the host via the wired backend.
func RestageCredentials(dir string) (bool, error) { return active.RestageCredentials(dir) }

// HostTokenExpiry reports the wired backend's host token expiry and whether it is usable.
func HostTokenExpiry() (int64, bool) { return active.HostTokenExpiry() }

// ContextUsage reports the wired backend's context size, window and model for the session under home.
func ContextUsage(home string) (int, int, string, bool) { return active.ContextUsage(home) }

// CompactionThreshold reports the wired backend's compaction threshold for a window this size.
func CompactionThreshold(window int) int { return active.CompactionThreshold(window) }

// ModelWindow resolves model to its window via the wired backend.
func ModelWindow(model string) (int, bool) { return active.ModelWindow(model) }

// ModelForTier resolves tier to a model via the wired backend.
func ModelForTier(tier string) (string, bool) { return active.ModelForTier(tier) }

// ToolRunning reports whether the wired backend reads screen as a tool call in flight.
func ToolRunning(screen string) bool { return active.ToolRunning(screen) }

// noop is the default until Use: state is Unknown, no home is provisioned.
type noop struct{}

func (noop) DetectState(string) State { return Unknown }

func (noop) PrepareHome(HomeSpec) (Home, error) { return Home{}, nil }

func (noop) RestageCredentials(string) (bool, error) { return false, nil }

func (noop) HostTokenExpiry() (int64, bool) { return 0, false }

func (noop) ContextUsage(string) (int, int, string, bool) { return 0, 0, "", false }

func (noop) CompactionThreshold(int) int { return math.MaxInt } // never worth it: nothing to measure

func (noop) ModelWindow(string) (int, bool) { return 0, false } // nothing wired, nothing recognised

func (noop) ModelForTier(string) (string, bool) { return "", false } // nothing wired, nothing recognised

func (noop) ToolRunning(string) bool { return false }
