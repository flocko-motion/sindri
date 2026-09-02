// package: hub/workflow / fullness
// type:    logic (how full a worker's context is, and what that gates)
// job:     the hard-stop fullness fraction (ContextFullFraction) and an already-claimed
// assignment's own preparation — a worker's is a model switch, else a clear, else compaction
// (prepareAssignment); a reviewer's is always the clear (clearForFreshStart).
// limits:  the fill facts and firing preparation; the claim itself is the caller's.
package workflow

import "context"

// ContextFullFraction is how much of its window a worker may fill before a fresh assignment
// clears it rather than compacting it — a session that far gone is not worth summarizing.
const ContextFullFraction = 0.85

// contextFull reports whether a worker is past ContextFullFraction of its window. A window of 0,
// or no recorded usage yet, is never full — guessing one is what this replaced.
func (e *Engine) contextFull(project, worker string) (tokens int, full bool) {
	o := e.hn.Observe(project, worker)
	return o.Fill, o.Window > 0 && float64(o.Fill) >= float64(o.Window)*ContextFullFraction
}

// compactDue is fill past CompactionThreshold's curve — a trigger, not the hard stop.
func (e *Engine) compactDue(project, worker string) (tokens int, due bool) {
	o := e.hn.Observe(project, worker)
	return o.Fill, o.Window > 0 && o.Fill >= e.hn.CompactionThreshold(o.Window)
}

// compactIfDue fires a due compaction once, queuing dir — the real instruction — behind it, so the
// agent is never handed dir to act on and then cut off by the compaction that follows.
func (e *Engine) compactIfDue(ctx context.Context, project, agent, dir string) (fired bool, err error) {
	if _, due := e.compactDue(project, agent); !due {
		return false, nil
	}
	if err := e.hn.Compact(ctx, project, agent); err != nil {
		return false, err
	}
	if err := e.hn.Say(project, agent, dir, PushOnly); err != nil {
		return false, err
	}
	return true, nil
}

// clearForFreshStart discards an agent's session before it is handed work: a worker's new task or
// new round, a reviewer's next PR. Work arrives whole — the directive carries the task, the branch
// and the feedback — so the previous unit is context to drop rather than condense.
//
// It takes a positive reading to fire, as SetModel's own guard does: nothing recorded is a session
// nobody has read yet — a just-launched one, most often — and /clear on an empty session leaves
// awaitCleared waiting out its cap for a drop that cannot come.
func (e *Engine) clearForFreshStart(ctx context.Context, project, agent string) (fired bool, err error) {
	if e.hn.Observe(project, agent).Fill == 0 {
		return false, nil
	}
	return true, e.hn.Clear(ctx, project, agent)
}

// prepareAssignment runs an already-claimed assignment's preparation — a model switch, else the
// clear — then delivers dir once it has landed. A switch clears on its own way through (-> SetModel).
func (e *Engine) prepareAssignment(ctx context.Context, project, agent, tier, dir string) (fired bool, err error) {
	if want, known := e.deps.ModelForTier(tier); known && !e.hn.ModelMatches(want, e.hn.Observe(project, agent).Model) {
		if err := e.hn.SetModel(ctx, project, agent, want); err != nil {
			return false, err
		}
		if err := e.hn.Say(project, agent, dir, PushOnly); err != nil {
			return false, err
		}
		return true, nil
	}
	cleared, err := e.clearForFreshStart(ctx, project, agent)
	if err != nil || !cleared {
		return false, err
	}
	return true, e.hn.Say(project, agent, dir, PushOnly)
}
