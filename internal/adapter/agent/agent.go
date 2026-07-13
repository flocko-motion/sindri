// package: adapter/agent / agent
// type:    logic (the coding-agent PORT — hexagonal abstraction)
// job:     the contract sindri needs from a coding agent, abstracting the tool
//
//	(Claude Code today): classify a rendered pane's runtime state, and
//	provision an agent's home from a workflow-composed prompt. The hub
//	talks to this port; a backend (adapter/agent/claude, future others)
//	implements it, wired once via Use.
//
// limits:  no agent specifics here (-> adapter/agent/claude, which depends on this
//
//	package, never the reverse).
package agent

import "io"

// State is a coding agent's detected runtime state (distinct from sindri's workflow
// phase — this is what the agent tool itself is doing right now).
type State string

const (
	Working State = "working" // actively processing a turn
	Blocked State = "blocked" // waiting for a user response
	Idle    State = "idle"    // stopped at the prompt, nothing happening
	Unknown State = "unknown" // not classifiable (shell, transcript viewer, boot, …)
)

// HomeSpec is what a backend needs to provision one agent's home: where it lives and
// the already-composed system prompt to persist. The workflow composes the prompt
// (it's high-level logic); the backend only writes the tool-specific files it needs
// (credentials, config, settings) around it.
type HomeSpec struct {
	Dir          string    // host home dir to create + populate (mounted into the pod)
	SystemPrompt string    // composed by the workflow; the backend persists it verbatim
	Out          io.Writer // setup announcements (e.g. a one-time credential-access prompt)
}

// Home is a provisioned agent home ready to mount: the home dir and its sibling
// config file (host paths), plus whether host credentials were found — no creds means
// the caller can't run the agent authenticated.
type Home struct {
	Dir        string
	ConfigPath string
	HasCreds   bool
}

// Agent is the port: what the hub needs from a coding-agent backend.
type Agent interface {
	// DetectState classifies the agent's runtime state from its rendered pane text
	// (as `tmux capture-pane -p` yields).
	DetectState(screen string) State
	// PrepareHome provisions the agent's home under spec.Dir (credentials, config,
	// and the composed system prompt) and returns the host paths to mount.
	PrepareHome(spec HomeSpec) (Home, error)
}

// active is the backend this process runs against, wired once at startup by the
// composition root via Use. Defaults to a no-op so the port is safe before Use.
var active Agent = noop{}

// Use selects the coding-agent backend for this process. Called once at startup.
func Use(a Agent) { active = a }

// DetectState classifies a pane via the wired backend.
func DetectState(screen string) State { return active.DetectState(screen) }

// PrepareHome provisions an agent home via the wired backend.
func PrepareHome(spec HomeSpec) (Home, error) { return active.PrepareHome(spec) }

// noop is the default until Use wires a real backend: state is always Unknown and
// no home can be provisioned.
type noop struct{}

func (noop) DetectState(string) State { return Unknown }

func (noop) PrepareHome(HomeSpec) (Home, error) { return Home{}, nil }
