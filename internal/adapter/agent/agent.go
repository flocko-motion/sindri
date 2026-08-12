// package: adapter/agent / agent
// type:    logic (the coding-agent PORT — hexagonal abstraction)
// job:     the contract sindri needs from a coding agent (Claude Code today): classify a pane's
// runtime state, and provision an agent's home from a composed prompt. A backend implements it.
// limits:  no agent specifics here (-> adapter/agent/claude, which depends on this
// package, never the reverse).
package agent

import "io"

// State is what the agent tool is doing now, not sindri's workflow phase.
type State string

const (
	Working State = "working" // actively processing a turn
	Blocked State = "blocked" // waiting for a user response
	Idle    State = "idle"    // stopped at the prompt, nothing happening
	// SignedOut: the tool has no valid credentials, so it cannot run a turn at all. Distinct from
	// Blocked, which a message answers — nothing typed at a signed-out prompt is ever sent.
	SignedOut State = "signed-out"
	Unknown   State = "unknown" // not classifiable (shell, transcript viewer, boot, …)
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
	// ContextUsage reports what the live session under home carries and the window it fills, both
	// from the backend's own transcript. The window comes from here because only the backend knows
	// which model answers; a caller that assumed one retired workers with most of 1M unused.
	// ok=false when nothing has been recorded yet.
	ContextUsage(home string) (tokens, window int, ok bool)
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
	case Working, Blocked, SignedOut:
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

// ContextUsage reports the wired backend's context size and window for the session under home.
func ContextUsage(home string) (int, int, bool) { return active.ContextUsage(home) }

// noop is the default until Use: state is Unknown, no home is provisioned.
type noop struct{}

func (noop) DetectState(string) State { return Unknown }

func (noop) PrepareHome(HomeSpec) (Home, error) { return Home{}, nil }

func (noop) RestageCredentials(string) (bool, error) { return false, nil }

func (noop) HostTokenExpiry() (int64, bool) { return 0, false }

func (noop) ContextUsage(string) (int, int, bool) { return 0, 0, false }
