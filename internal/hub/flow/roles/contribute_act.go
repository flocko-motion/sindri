// package: hub/flow/roles / contribute_act
// type:    logic (mid-task contribution: interim PR)
// job:     CmdContribute — a worker lands work MID-task, without finishing it: queue its
// quality gate and reply at once. What passing it lands — commit, rebase, an
// interim PR or a feature milestone — is hub/flow/pr's (-> OpenMilestoneOrInterim).
// limits:  the pre-gate checks and the queue hand-off only.
package roles

import (
	"fmt"
	"github.com/flo-at/sindri/internal/hub/flow/run"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"io"
	"strings"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// CmdContribute queues an interim contribution's quality gate and returns at once — a worker
// inside a feature contributes the whole branch as a milestone once it passes, where the wait
// for others to see the work is longest.
func (a *Act) CmdContribute(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := a.Store.For(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	// Inside a feature what is worth landing is the branch, not the subtask in hand — which is the
	// operation the user's milestone trigger already performs, so this is the same door for the agent.
	if st.Container == "" && (st.Phase != "working" || st.Task == "") {
		fmt.Fprintln(out, prompts.ReplyNotWorking("contribute", st.Phase, st.Task))
		return 1, nil
	}
	// The gate is queued, not run here (sd-cf630b) — see CmdSubmit for why, including why the work
	// is committed and the agent parked before it opens. No PR exists until the gate passes.
	msg := strings.TrimSpace(strings.Join(args, " "))
	sha, err := a.pr().GateCommit(c.Project, c.Agent, msg)
	if err != nil {
		return 1, err
	}
	if err := ps.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "gating"},
		store.ReasonAdvanced, "contribute queued: "+sha); err != nil {
		return 1, err
	}
	qr, reused, err := a.pr().GateRun(c.Project, c.Agent, run.GateContribute, msg, sha)
	if err != nil {
		_ = ps.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "working"},
			store.ReasonAdvanced, "contribute gate could not be opened")
		return 1, err // see CmdSubmit: "gating" is a park with no way out if no gate was opened
	}
	if reused {
		fmt.Fprintln(out, prompts.ReplyGateReused(qr.ID, prompts.ShortSHA(sha)))
		return 0, nil
	}
	fmt.Fprintln(out, prompts.ReplyGateQueued(qr.ID, a.pr().QueuePosition(qr.ID)))
	return 0, nil
}
