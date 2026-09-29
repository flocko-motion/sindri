// package: hub/flow/fleet / kickoff
// type:    logic (what a fresh session is told, unprompted)
// job:     the words the hub speaks first when an agent's session comes up — its directive, in the
// state it is actually in, so a launch needs no round trip to find out what the hub already knows.
// limits:  the wording. Delivery is the hub's (-> Hub.greet, harness.Service.Clear), and the words
// per state are flowsay.go's.
package fleet

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/hub/prompts"
)

// Kickoff is what an agent's fresh session is told: who it is, and where it stands right now. It
// SERVES the directive rather than sending the agent to fetch one — that round trip fired eight
// times in one session to say "carry on with the user", and told an agent to ask for an answer the
// hub was holding as it spoke.
func (e *Engine) Kickoff(project, name string) string {
	ps := e.Store.For(project)
	ag, found, err := ps.GetAgent(name)
	if err != nil || !found {
		return e.untold(project, name, fmt.Errorf("no agent %q is on this project's roster: %v", name, err))
	}
	st, _ := ps.GetState(name)
	// Escalated outranks the role here as it does in directive, and speaks as the hub already — so it
	// stands alone rather than inside the role's framing.
	if st.Escalation != "" {
		return prompts.DirEscalated(st.Escalation)
	}
	dir, err := e.stands(project, name)
	if err != nil {
		return e.untold(project, name, err)
	}
	unread, _ := ps.UnreadMailCount(name)
	return prompts.MsgKickoff(ag.Role, e.prAct().RebaseNotice(project, name)+dir, unread)
}

// untold is what a session gets when the hub cannot work out where its agent stands. It names the
// fault and leaves a trail: a fresh session told nothing at all would sit there, and one told to
// carry on would be acting on a state nobody could read.
func (e *Engine) untold(project, name string, err error) string {
	fmt.Fprintf(os.Stderr, "hub: kickoff for %s/%s: %v\n", project, name, err)
	_ = e.Store.For(project).Log(name, "kickoff-failed", err.Error())
	return fmt.Sprintf("[hub] Your session is up, but the hub cannot say where you stand: %v. "+
		"Do not start work on a guess — `sindri escalate \"<what you need>\"` puts this in front "+
		"of the user.", err)
}
