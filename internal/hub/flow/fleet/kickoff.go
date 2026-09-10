// package: hub/flow/fleet / kickoff
// type:    logic (what a fresh session is told, unprompted)
// job:     resolve the one line the hub speaks first when an agent's session comes up — a launch,
// or a context clear. A role the hub holds a job for is sent to `sindri`; a planner and a
// coauthor are handed the directive here, since the hub already knows what it will say.
// limits:  the choice only. The strings are injected.go's and prompts.go's, and the delivery is
// the hub's (-> Hub.rehydrate, agent.Service.Clear).
package fleet

import "github.com/flo-at/sindri/internal/hub/prompts"

// Kickoff is what an agent's fresh session is told. A planner and a coauthor are served the
// directive rather than sent to fetch it: that round trip fired eight times in one session to say
// "carry on with the user", and a coauthor asking the hub for work has nothing to ask it for.
func (e *Engine) Kickoff(project, name string) string {
	ps := e.Store.For(project)
	ag, found, err := ps.GetAgent(name)
	if err != nil || !found {
		return prompts.MsgKickoff
	}
	st, _ := ps.GetState(name)
	dir, standing := standingDirective(ag.Role, st)
	if !standing {
		return prompts.MsgKickoff
	}
	// Escalated outranks the role here as it does in directive, and speaks as the hub already — so it
	// stands alone rather than inside the role's framing.
	if st.Escalation != "" {
		return prompts.DirEscalated(st.Escalation)
	}
	unread, _ := ps.UnreadMailCount(name)
	return prompts.MsgStandingKickoff(ag.Role, e.prAct().RebaseNotice(project, name)+dir, unread)
}
