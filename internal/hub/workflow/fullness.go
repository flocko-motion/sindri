// package: hub/workflow / fullness
// type:    logic (how full a worker's context is, and what that gates)
// job:     the hard-stop fullness fraction (ContextFullFraction) and an already-claimed
// assignment's own preparation (prepareAssignment) — a model switch, else a clear once a worker
// is too far gone to summarize, else compaction if merely due.
// limits:  the fill facts and firing preparation; the claim itself is the caller's.
package workflow

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
func (e *Engine) compactIfDue(project, agent, dir string) (fired bool, err error) {
	if _, due := e.compactDue(project, agent); !due {
		return false, nil
	}
	return true, e.deps.Compact(project, agent, dir)
}

// prepareAssignment runs an already-claimed assignment's preparation — a model switch, else a clear
// past ContextFullFraction, else compaction if merely due — bracketed so AtLeafBoundary admits the claim.
func (e *Engine) prepareAssignment(project, agent, tier, dir string) (fired bool, err error) {
	e.deps.BeginAssignment(project, agent)
	defer e.deps.EndAssignment(project, agent)
	if want, known := e.deps.ModelForTier(tier); known && !e.deps.ModelMatches(want, e.deps.CurrentModel(project, agent)) {
		return true, e.deps.SetModel(project, agent, want, dir)
	}
	if _, full := e.contextFull(project, agent); full {
		return true, e.deps.FireClear(project, agent, dir, false)
	}
	return e.compactIfDue(project, agent, dir)
}
