// package: hub/flow/pr / contribute_act
// type:    logic (mid-task contribution: interim PR)
// job:     CmdContribute — a worker lands work MID-task, without finishing it: queue its
// quality gate and reply at once. What passing it lands — commit, rebase, an
// interim PR or a feature milestone — is this package's too (-> OpenMilestoneOrInterim).
// limits:  the pre-gate checks and the queue hand-off only.
package pr

import (
	"fmt"
	"github.com/flo-at/sindri/internal/hub/world/situation"
	"io"
	"strings"

	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/flow/run"
	"github.com/flo-at/sindri/internal/hub/prompts"
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
	if st.Container == "" && (!situation.Standing(st.Phase, "working") || st.Task == "") {
		fmt.Fprintln(out, prompts.ReplyNotWorking("contribute", st.Phase, st.Task))
		return 1, nil
	}
	// The gate is queued, not run here (sd-cf630b) — see CmdSubmit for why the work is committed
	// before it opens. No PR exists until the gate passes.
	msg := strings.TrimSpace(strings.Join(args, " "))
	sha, err := a.GateCommit(c.Project, c.Agent, msg)
	if err != nil {
		return 1, err
	}
	// The agent is moved nowhere: it keeps the task and the branch through a contribution, and where
	// that leaves it is decided by what the gate then lands (-> worker/working's own exits).
	qr, reused, err := a.GateRun(c.Project, c.Agent, run.GateContribute, msg, sha)
	if err != nil {
		return 1, err
	}
	if reused {
		fmt.Fprintln(out, prompts.ReplyGateReused(qr.ID, prompts.ShortSHA(sha)))
		return 0, nil
	}
	fmt.Fprintln(out, prompts.ReplyGateQueued(qr.ID, a.QueuePosition(qr.ID)))
	return 0, nil
}
