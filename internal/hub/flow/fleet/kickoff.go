// package: hub/flow/fleet / kickoff
// type:    logic (what a fresh session is told, unprompted)
// job:     resolve the one line the hub speaks first when an agent's session comes up — a launch,
// or a context clear. A role the hub holds a job for is sent to `sindri`; a planner is
// handed its directive here, since the hub already knows what that answer will be.
// limits:  the choice only. The strings are injected.go's and prompts.go's, and the delivery is
// the hub's (-> Hub.rehydrate, agent.Service.Clear).
package fleet

import "github.com/flo-at/sindri/internal/hub/prompts"

// Kickoff is what an agent's fresh session is told. A planner is served its directive rather than
// sent to fetch it: that round trip fired eight times in one session to say "carry on with the user".
func (e *Engine) Kickoff(project, name string) string {
	ps := e.Store.For(project)
	ag, ok, err := ps.GetAgent(name)
	if err != nil || !ok || ag.Role != "planner" {
		return prompts.MsgKickoff
	}
	st, _ := ps.GetState(name)
	// Escalated outranks the role here as it does in directive, and speaks as the hub already — so it
	// stands alone rather than inside the planner framing.
	if st.Escalation != "" {
		return prompts.DirEscalated(st.Escalation)
	}
	unread, _ := ps.UnreadMailCount(name)
	return prompts.MsgPlannerKickoff(e.prAct().RebaseNotice(project, name)+plannerDirective(st), unread)
}
