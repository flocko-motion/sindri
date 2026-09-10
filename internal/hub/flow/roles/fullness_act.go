// package: hub/flow/roles / fullness_act
// type:    logic (how full a worker's context is, and what that gates)
// job:     the hard-stop fullness fraction (ContextFullFraction) and an already-claimed
// assignment's own preparation — a worker's is a model switch, else a clear, else compaction
// (PrepareAssignment); a reviewer's is always the clear (ClearForFreshStart).
// limits:  the fill facts and firing preparation; the claim itself is the caller's.
package roles

import (
	"context"
	"fmt"
	"github.com/flo-at/sindri/internal/hub/core"
	"os"
)

// ContextFullFraction is how much of its window a worker may fill before a fresh assignment
// clears it rather than compacting it — a session that far gone is not worth summarizing.
const ContextFullFraction = 0.85

// ContextFull reports whether a worker is past ContextFullFraction of its window. A window of 0,
// or no recorded usage yet, is never full — guessing one is what this replaced.
func (a *Act) ContextFull(project, worker string) (tokens int, full bool) {
	o := a.Harness.Observe(project, worker)
	return o.Fill, o.Window > 0 && float64(o.Fill) >= float64(o.Window)*ContextFullFraction
}

// CompactDue is fill past CompactionThreshold's curve — a trigger, not the hard stop.
func (a *Act) CompactDue(project, worker string) (tokens int, due bool) {
	o := a.Harness.Observe(project, worker)
	return o.Fill, o.Window > 0 && o.Fill >= a.Harness.CompactionThreshold(o.Window)
}

// CompactIfDue fires a due compaction once, queuing dir — the real instruction — behind it, so the
// agent is never handed dir to act on and then cut off by the compaction that follows.
func (a *Act) CompactIfDue(ctx context.Context, project, agent, dir string) (fired bool, err error) {
	if _, due := a.CompactDue(project, agent); !due {
		return false, nil
	}
	if err := a.Harness.Compact(ctx, project, agent); err != nil {
		return false, err
	}
	if err := a.Harness.Say(project, agent, dir, core.PushOnly); err != nil {
		return false, err
	}
	return true, nil
}

// ClearForFreshStart discards an agent's session before it is handed work. Work arrives whole, so
// the previous unit is context to drop rather than condense. A positive reading is required to fire:
// /clear on an empty session leaves awaitCleared waiting out its cap for a drop that cannot come.
func (a *Act) ClearForFreshStart(ctx context.Context, project, agent string) (fired bool, err error) {
	if a.Harness.Observe(project, agent).Fill == 0 {
		return false, nil
	}
	return true, a.Harness.Clear(ctx, project, agent)
}

// PrepareAssignment runs an already-claimed assignment's preparation — a model switch, else the
// clear — then delivers dir once it has landed. It runs INSIDE the hand-over: the whole of it is one
// state, so an event arriving mid-way cancels the hand-over rather than catching it between two
// states (-> hub/flow/roles/worker's assigning).
func (a *Act) PrepareAssignment(ctx context.Context, project, agent, tier, dir string) (fired bool, err error) {
	if want, known := a.Deps.ModelForTier(tier); known && !a.Harness.ModelMatches(want, a.Harness.Observe(project, agent).Model) {
		if err := a.Harness.SetModel(ctx, project, agent, want); err != nil {
			return false, err
		}
		return true, a.Harness.Say(project, agent, dir, core.PushOnly)
	}
	cleared, clearErr := a.ClearForFreshStart(ctx, project, agent)
	if clearErr != nil {
		// A clear that never lands must not withhold the work: awaitCleared gives up after
		// clearSettleCap, and 4 of jari's 5 model switches died there. A crowded session beats none.
		fmt.Fprintf(os.Stderr, "hub: clearing %s before its next work: %v\n", agent, clearErr)
		_ = a.Store.For(project).Log(agent, "prepare-unprepared", "handed work without a clear: "+clearErr.Error())
		return false, nil
	}
	if !cleared {
		return false, nil
	}
	return true, a.Harness.Say(project, agent, dir, core.PushOnly)
}
