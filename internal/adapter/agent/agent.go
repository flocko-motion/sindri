// package: adapter/agent / agent
// type:    logic (the coding-agent PORT — hexagonal abstraction)
// job:     the contract sindri needs from a coding agent, abstracting the tool
//          (Claude Code today): classify a rendered pane's runtime state. The hub
//          talks to this port; a backend (adapter/agent/claude, future others)
//          implements it, wired once via Use.
// limits:  no agent specifics here (-> adapter/agent/claude, which depends on this
//          package, never the reverse).
package agent

// State is a coding agent's detected runtime state (distinct from sindri's workflow
// phase — this is what the agent tool itself is doing right now).
type State string

const (
	Working State = "working" // actively processing a turn
	Blocked State = "blocked" // waiting for a user response
	Idle    State = "idle"    // stopped at the prompt, nothing happening
	Unknown State = "unknown" // not classifiable (shell, transcript viewer, boot, …)
)

// Agent is the port: what the hub needs from a coding-agent backend.
type Agent interface {
	// DetectState classifies the agent's runtime state from its rendered pane text
	// (as `tmux capture-pane -p` yields).
	DetectState(screen string) State
}

// active is the backend this process runs against, wired once at startup by the
// composition root via Use. Defaults to a no-op so the port is safe before Use.
var active Agent = noop{}

// Use selects the coding-agent backend for this process. Called once at startup.
func Use(a Agent) { active = a }

// DetectState classifies a pane via the wired backend.
func DetectState(screen string) State { return active.DetectState(screen) }

// noop is the default until Use wires a real backend: state is always Unknown.
type noop struct{}

func (noop) DetectState(string) State { return Unknown }
