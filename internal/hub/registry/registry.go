// package: hub/registry / registry
// type:    logic (the state-filtered command surface — the "browser" menu)
// job:     define the set of hub-side verbs an agent may run and filter them by the
// caller's role and state, so the listed surface is only what is valid now.
// The heart of the browser design (D-hub): `GET /commands` returns
// Available(caller), and a verb held back still answers with why (-> Resolve).
// limits:  single-owner (only the hub builds/serves a Registry); pure types +
// filtering. The Run closures are supplied by the hub (no hub import).
package registry

import (
	"io"
	"slices"
)

// Caller is who is asking: their identity, role, and (from Phase 3) workflow
// state. The registry filters the surface against this.
type Caller struct {
	Project   string // the repo (repoTag) the caller belongs to
	Agent     string
	Role      string // "worker" | "reviewer"
	HasTask   bool   // a worker holding work (a leaf task OR a container) hides "next"
	Container string // the feature (parent task) it holds, if any: shows "checkpoint" alongside submit
	// SubtasksOpen: the held feature still has children to work, which is what holds "submit" back —
	// a feature goes up when its last subtask is checkpointed, not when a human decides to cut it.
	SubtasksOpen bool
	Task         string // the task or subtask it holds; carried so a blocked verb can name the work
	Phase        string // the agent's current phase (working|submitted|resolving|idle|…), gating phase-specific verbs
	InChat       bool   // a member of the user's chatroom: shows "chat"
	// Escalation is the question this agent stopped on, waiting for the user ("" = not escalated).
	// The QUESTION rather than a flag, so a verb held back by it answers in the agent's own words —
	// the state is one the agent asked for, and hearing its own question back is what identifies it.
	Escalation string
}

// Command is one hub-side verb the browser can invoke.
type Command struct {
	Name string
	Help string
	// HelpFor tailors the help line to the caller, for a verb whose ARGUMENTS differ by role or
	// state — what a worker types for `comment` is not what a planner types. The surface is already
	// filtered to what a caller can do, and by the same reasoning an argument a caller never supplies
	// does not belong in the one line it reads to learn the verb. nil means Help serves everyone.
	HelpFor func(Caller) string
	// Roles allowed to see/run this command; empty means all roles.
	Roles []string
	// Blocked reports why the state machine holds this command back from a caller right now, or ""
	// when it may run. The REASON is the field rather than a boolean because it is handed to the
	// agent verbatim when it asks for the verb anyway: a state gate that cannot say what to do
	// instead leaves the agent to guess the workflow. It must name the verb that IS open to it.
	// nil means "always available to its roles".
	Blocked func(Caller) string
	// Run executes the command, streaming to out, returning a process-style exit
	// code. Supplied by the hub so it can reach the store/adapters.
	Run func(c Caller, args []string, out io.Writer) (int, error)
}

// HelpText is the help this caller should read: the tailored line where the verb has one, else Help.
// Every reader of a command's help goes through here, so a tailored verb cannot be listed with one
// wording and answer `--help` with another.
func (cmd Command) HelpText(c Caller) string {
	if cmd.HelpFor != nil {
		return cmd.HelpFor(c)
	}
	return cmd.Help
}

// Available reports whether cmd is offered to caller right now.
func (cmd Command) Available(c Caller) bool {
	if len(cmd.Roles) > 0 && !slices.Contains(cmd.Roles, c.Role) {
		return false
	}
	return cmd.Blocked == nil || cmd.Blocked(c) == ""
}

// Registry is an ordered set of commands.
type Registry struct {
	cmds  []Command
	index map[string]Command
}

// New builds a registry from commands (registration order is preserved for
// stable listing).
func New(cmds ...Command) *Registry {
	r := &Registry{index: make(map[string]Command, len(cmds))}
	for _, c := range cmds {
		r.cmds = append(r.cmds, c)
		r.index[c.Name] = c
	}
	return r
}

// Available returns the commands offered to caller now, in registration order.
func (r *Registry) Available(c Caller) []Command {
	var out []Command
	for _, cmd := range r.cmds {
		if cmd.Available(c) {
			out = append(out, cmd)
		}
	}
	return out
}

// Offered is one verb of the caller's role and, when the state machine is holding it back, why.
type Offered struct {
	Command
	Blocked string // "" when it can be run now
}

// Surface is every verb of the caller's ROLE, each with the reason it cannot be run yet. Listed
// rather than hidden: an agent reads this as what EXISTS, so a missing verb reads as one the system
// never had — one shown a list without submit concluded the hub was broken. Role isolation still
// applies, so another role's verbs are absent entirely rather than blocked.
func (r *Registry) Surface(c Caller) []Offered {
	var out []Offered
	for _, cmd := range r.cmds {
		if len(cmd.Roles) > 0 && !slices.Contains(cmd.Roles, c.Role) {
			continue
		}
		var why string
		if cmd.Blocked != nil {
			why = cmd.Blocked(c)
		}
		out = append(out, Offered{Command: cmd, Blocked: why})
	}
	return out
}

// Resolve answers what a caller may do with a verb: the command when it is open to it, otherwise the
// reason the state machine is holding it back. A ROLE mismatch reports no reason and so reads as an
// unknown name — a worker learns nothing of the reviewer's surface — but a verb of its own role at
// the wrong moment explains itself, because the agent has to be told which verb to reach for instead.
func (r *Registry) Resolve(name string, c Caller) (cmd Command, reason string, ok bool) {
	cmd, exists := r.index[name]
	if !exists || (len(cmd.Roles) > 0 && !slices.Contains(cmd.Roles, c.Role)) {
		return Command{}, "", false
	}
	if cmd.Blocked != nil {
		if reason := cmd.Blocked(c); reason != "" {
			return Command{}, reason, false
		}
	}
	return cmd, "", true
}
