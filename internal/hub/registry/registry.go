// package: hub/registry / registry
// type:    logic (the state-filtered command surface — the "browser" menu)
// job:     define the set of hub-side verbs an agent may run and filter them by
//          the caller's role and state, so a command that is not currently valid
//          is invisible rather than rejected. This filter is the heart of the
//          browser design (D-hub): `GET /commands` returns Available(caller).
// limits:  single-owner (only the hub builds/serves a Registry); pure types +
//          filtering. The Run closures are supplied by the hub (no hub import).
package registry

import (
	"io"
	"slices"
)

// Caller is who is asking: their identity, role, and (from Phase 3) workflow
// state. The registry filters the surface against this.
type Caller struct {
	Project     string // the repo (repoTag) the caller belongs to
	Agent       string
	Role        string // "worker" | "reviewer"
	HasTask     bool   // a worker holding work (a leaf task OR a container) hides "next"
	InContainer bool   // a worker holding a collaborative container: shows "checkpoint", hides "submit"
}

// Command is one hub-side verb the browser can invoke.
type Command struct {
	Name string
	Help string
	// Roles allowed to see/run this command; empty means all roles.
	Roles []string
	// Hidden reports whether the command is currently unavailable for a caller
	// beyond the role check (state machine). nil means "always available to its
	// roles".
	Hidden func(Caller) bool
	// Run executes the command, streaming to out, returning a process-style exit
	// code. Supplied by the hub so it can reach the store/adapters.
	Run func(c Caller, args []string, out io.Writer) (int, error)
}

// Available reports whether cmd is offered to caller right now.
func (cmd Command) Available(c Caller) bool {
	if len(cmd.Roles) > 0 && !slices.Contains(cmd.Roles, c.Role) {
		return false
	}
	if cmd.Hidden != nil && cmd.Hidden(c) {
		return false
	}
	return true
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

// Lookup finds a command by name and reports whether it is available to caller.
// A command that exists but is not available to the caller returns ok=false, so
// an out-of-surface verb is indistinguishable from an unknown one.
func (r *Registry) Lookup(name string, c Caller) (Command, bool) {
	cmd, exists := r.index[name]
	if !exists || !cmd.Available(c) {
		return Command{}, false
	}
	return cmd, true
}
