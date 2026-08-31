// package: hub/workflow / fullness
// type:    logic (how full a worker's context is, and what that gates)
// job:     the hard-stop fullness fraction (ContextFullFraction) and an already-claimed
// assignment's own preparation (prepareAssignment) — a model switch, else a clear once a worker
// is too far gone to summarize, else compaction if merely due.
// limits:  the fill facts and firing preparation; the claim itself is the caller's.
package workflow

import "context"

// ContextFullFraction is how much of its window a worker may fill before a fresh assignment
// clears it rather than compacting it — a session that far gone is not worth summarizing.
const ContextFullFraction = 0.85

// contextFull reports whether a worker is past ContextFullFraction of its window. A window of 0,
// or no recorded usage yet, is never full — guessing one is what this replaced.
func (e *Engine) contextFull(project, worker string) (tokens int, full bool) {
	tokens, window, _, ok := e.deps.ContextUsage(project, worker)
	return tokens, ok && window > 0 && float64(tokens) >= float64(window)*ContextFullFraction
}

// compactDue is fill past CompactionThreshold's curve — a trigger, not the hard stop.
func (e *Engine) compactDue(project, worker string) (tokens int, due bool) {
	tokens, window, _, ok := e.deps.ContextUsage(project, worker)
	return tokens, ok && window > 0 && tokens >= e.deps.CompactionThreshold(window)
}

// compactIfDue fires a due compaction once, queuing dir — the real instruction — behind it, so the
// agent is never handed dir to act on and then cut off by the compaction that follows.
func (e *Engine) compactIfDue(ctx context.Context, project, agent, dir string) (fired bool, err error) {
	if _, due := e.compactDue(project, agent); !due {
		return false, nil
	}
	if err := e.deps.Compact(ctx, project, agent); err != nil {
		return false, err
	}
	if err := e.deps.Deliver(project, agent, dir, PushOnly); err != nil {
		return false, err
	}
	return true, nil
}

// prepareAssignment runs an already-claimed assignment's preparation — a model switch, else a clear
// past ContextFullFraction, else compaction if merely due — then delivers dir once it has landed.
func (e *Engine) prepareAssignment(ctx context.Context, project, agent, tier, dir string) (fired bool, err error) {
	if want, known := e.deps.ModelForTier(tier); known && !e.deps.ModelMatches(want, e.deps.CurrentModel(project, agent)) {
		if err := e.deps.SetModel(ctx, project, agent, want); err != nil {
			return false, err
		}
		if err := e.deps.Deliver(project, agent, dir, PushOnly); err != nil {
			return false, err
		}
		return true, nil
	}
	if _, full := e.contextFull(project, agent); full {
		if err := e.deps.Clear(ctx, project, agent); err != nil {
			return false, err
		}
		err := e.deps.Deliver(project, agent, dir, PushOnly)
		return true, err
	}
	return e.compactIfDue(ctx, project, agent, dir)
}
