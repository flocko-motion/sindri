// package: hub/workflow / contribute
// type:    logic (mid-task contribution: interim PR)
// job:     CmdContribute — a worker lands work MID-task, without finishing it: queue its
// quality gate and reply at once. What passing it lands — commit, rebase, an
// interim PR or a feature milestone — is gate.go's (-> openMilestoneOrInterim).
// limits:  the pre-gate checks and the queue hand-off only.
package workflow

import (
	"fmt"
	"io"
	"strings"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// CmdContribute queues an interim contribution's quality gate and returns at once — a worker
// inside a feature contributes the whole branch as a milestone once it passes, where the wait
// for others to see the work is longest.
func (e *Engine) CmdContribute(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := e.store.For(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	// Inside a feature what is worth landing is the branch, not the subtask in hand — which is the
	// operation the user's milestone trigger already performs, so this is the same door for the agent.
	if st.Container == "" && (st.Phase != "working" || st.Task == "") {
		fmt.Fprintln(out, ReplyNotWorking("contribute", st.Phase, st.Task))
		return 1, nil
	}
	// The gate is queued, not run here (sd-cf630b) — see CmdSubmit for why. Nothing lands (no
	// commit, no PR) until it passes; openMilestoneOrInterim runs the rest once it does.
	msg := strings.TrimSpace(strings.Join(args, " "))
	run, err := e.enqueueGate(c.Project, c.Agent, "contribute", msg)
	if err != nil {
		return 1, err
	}
	if err := ps.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "gating"}); err != nil {
		return 1, err
	}
	pos := 0
	if all, aerr := e.store.AllRuns("queued"); aerr == nil {
		pos = queuePositions(all)[run.ID]
	}
	fmt.Fprintln(out, ReplyGateQueued(run.ID, pos))
	return 0, nil
}
