// package: hub / escalate
// type:    logic (an agent stopped on a decision only the user can make)
// job:     raise, record and clear an escalation — the durable state, the question written
// where a later reader finds it (the activity log and the task's own thread), and the
// two verbs either side of it (`escalate`, `resume`), plus the user's clear from the host.
// limits:  the state and the recording; the gate that shuts the work verbs is the registry's
// (-> escalationBlocked), and the marker is the board's (-> api.AgentNeedsUser).
package hub

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// escalateUsage is what an escalation with no question is answered with — the argument is the whole
// point of the verb, so the refusal explains what to ask rather than restating the syntax.
const escalateUsage = "usage: escalate <what needs deciding>\n" +
	"  Stops you on a decision only the user can make, and tells them. The question is\n" +
	"  REQUIRED: an escalation without one says no more than that something is wrong, which\n" +
	"  is the part they can already see. Ask for what you need decided, in one line.\n" +
	"  While escalated you may still read (status, task, show, prs, log, git), but every verb\n" +
	"  that advances work is refused. `sindri resume` clears it once you have their answer."

// escalateHelp is what the command registry advertises for escalate.
const escalateHelp = "stop on a decision only the user can make, and tell them: escalate <what needs deciding>"

// resumeHelp is what the registry advertises for resume. It names the escalation it clears, since
// this is the half of the pair an agent reads while it is stuck.
const resumeHelp = "clear your escalation once the user has answered, and carry on: resume"

// cmdEscalate is the agent-facing `escalate <what needs deciding>` verb.
func (h *Hub) cmdEscalate(c registry.Caller, args []string, out io.Writer) (int, error) {
	question := strings.TrimSpace(strings.Join(args, " "))
	if question == "" {
		fmt.Fprintln(out, escalateUsage)
		return 2, nil
	}
	on, err := h.Escalate(c.Project, c.Agent, question)
	if err != nil {
		return 1, err
	}
	fmt.Fprintln(out, workflow.ReplyEscalationRaised(question, on))
	return 0, nil
}

// cmdResume is the agent-facing `resume` verb: the agent clears its own escalation, because only it
// knows whether it has understood the answer.
func (h *Hub) cmdResume(c registry.Caller, _ []string, out io.Writer) (int, error) {
	if err := h.Resume(c.Project, c.Agent, "resumed with the user's answer in hand"); err != nil {
		return 1, err
	}
	fmt.Fprintln(out, workflow.ReplyResumed)
	return 0, nil
}

// Escalate marks an agent as stopped on a decision the user must make, and records the question in
// both places a later reader looks: the agent's activity log, and — where it holds work — a comment
// on that task, which is what survives the session that raised it. Returns the task commented on
// ("" when it holds none). The state is written FIRST: a failure to record must not leave an agent
// believing it has escalated while nothing is holding its work verbs shut.
func (h *Hub) Escalate(project, name, question string) (string, error) {
	ps := h.store.For(project)
	if err := ps.SetEscalation(name, question); err != nil {
		return "", err
	}
	if err := ps.Log(name, "escalate", question); err != nil {
		return "", err
	}
	// The subtask it is on, else the feature it holds — the same target its own `comment` writes to
	// (-> commentTarget), so the question lands where the rest of its findings do.
	st, err := ps.GetState(name)
	if err != nil {
		return "", err
	}
	on := st.Task
	if on == "" {
		on = st.Container
	}
	if on != "" {
		if cerr := h.comments.Add(project, on, name, "escalated: "+question); cerr != nil {
			// Logged host-side, not fatal: the escalation stands and the activity log already carries
			// the question. Losing the comment must not cost the agent the state that stops it.
			fmt.Fprintf(os.Stderr, "hub: recording %s's escalation on %s failed: %v\n", name, on, cerr)
			on = ""
		}
	}
	h.notify()
	return on, nil
}

// Resume clears an agent's escalation and records why, whoever asked. The agent clears its own once
// it has the answer; the user clears one from the host, because an agent may be deleted, restarted,
// or simply wrong that it was blocked — and an escalation nobody can clear is a stuck agent by
// another name. Clearing one that was never raised is a no-op, so neither caller has to check first.
func (h *Hub) Resume(project, name, why string) error {
	ps := h.store.For(project)
	st, err := ps.GetState(name)
	if err != nil {
		return err
	}
	if st.Escalation == "" {
		return nil
	}
	if err := ps.ClearEscalation(name); err != nil {
		return err
	}
	if err := ps.Log(name, "resume", why); err != nil {
		return err
	}
	h.notify()
	return nil
}
