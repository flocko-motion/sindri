// package: hub/flow/agent / self
// type:    logic (what an agent asks about itself and its colleagues)
// job:     the three verbs that read rather than change anything — `status` (who am I, am I up),
// `log` (record a note on my own work), `staff` (who else is here and what do they hold).
// limits:  reading and the one note. Nothing here moves an agent, and who may run each is the
// catalogue's (-> hub/api/agents/verb).
package agent

import (
	"fmt"
	"io"
	"strings"

	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// CmdStatus tells an agent who it is and whether the hub can see its session.
func (a *Act) CmdStatus(c registry.Caller, _ []string, out io.Writer) (int, error) {
	// The standing observation, not a probe — and nothing seen YET is not "down": the caller is
	// running this from inside its own pod, so only a confirmed-down reading says otherwise.
	o := a.Harness.Observe(c.Project, c.Agent)
	running := !o.Seen() || o.Up
	fmt.Fprintf(out, "agent:   %s\nrole:    %s\nrunning: %v\n", c.Agent, c.Role, running)
	return 0, nil
}

// CmdLog records a note on the caller's own work, where a later reader of the activity log finds it.
func (a *Act) CmdLog(c registry.Caller, args []string, out io.Writer) (int, error) {
	msg := strings.TrimSpace(strings.Join(args, " "))
	if msg == "" {
		fmt.Fprintln(out, "usage: log <message>")
		return 2, nil
	}
	if err := a.Store.For(c.Project).Log(c.Agent, "note", msg); err != nil {
		return 1, err
	}
	fmt.Fprintln(out, "logged")
	return 0, nil
}

// CmdStaff lists the caller's colleagues in ITS OWN project — who they are and what each holds,
// the answer to "who do I ask about this". Scoped by construction, the roster query being: an
// agent may talk to anyone the user has named to it, and may not browse the fleet for a name.
func (a *Act) CmdStaff(c registry.Caller, _ []string, out io.Writer) (int, error) {
	ps := a.Store.For(c.Project)
	roster, err := ps.Roster()
	if err != nil {
		return 1, err
	}
	if len(roster) == 0 {
		fmt.Fprintln(out, "no agents in this repo yet")
		return 0, nil
	}
	for _, ag := range roster {
		who := ag.Name
		if ag.Name == c.Agent {
			who += " (you)"
		}
		fmt.Fprintf(out, "%-14s %-9s %s\n", who, ag.Role, staffHolding(a.Store, ag))
	}
	return 0, nil
}

// staffHolding is what one colleague has in hand: a reviewer a PR, everyone else a task or the
// feature it is working through. Retirement is said, since it decides whether to wait for them.
func staffHolding(fleet *store.Store, a store.Agent) string {
	var holding string
	if a.Role == "reviewer" {
		// fleet's ReviewingPR: a pooled reviewer's row is never filed under its own project.
		if _, pr, err := fleet.ReviewingPR(a.Project, a.Name); err == nil && pr != "" {
			holding = "reviewing " + pr
		}
	}
	st, err := fleet.For(a.Project).GetState(a.Name)
	if err == nil && holding == "" {
		switch {
		case st.Container != "" && st.Task != "":
			holding = st.Container + " › " + st.Task
		case st.Container != "":
			holding = st.Container
		case st.Task != "":
			holding = st.Task
		}
	}
	if holding == "" {
		holding = "nothing in hand"
	}
	if a.Retired {
		holding += " (retired: finishing what it holds, taking nothing new)"
	}
	return holding
}
