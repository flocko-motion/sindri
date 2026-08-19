// package: hub/workflow / fullness
// type:    logic (how full a worker's context is, and what that gates)
// job:     the hard-stop fullness fraction (ContextFullFraction), the lower compaction bar
// (compactDue), and an already-claimed assignment's own preparation (prepareAssignment).
// limits:  the fill facts and firing preparation; the claim itself is the caller's.
package workflow

// ContextFullFraction is how much of its window a worker may fill before it stops being handed new
// work — a flat token count would retire workers early on a large window.
const ContextFullFraction = 0.85

// Full is the fullness rule over figures the caller already holds (the board's own sample). A
// window of 0 is never full; guessing one is what this replaced.
func Full(tokens, window int) bool {
	return window > 0 && float64(tokens) >= float64(window)*ContextFullFraction
}

// contextFull is the same fact for a caller with no reading of its own — the assignment gate.
func (e *Engine) contextFull(project, worker string) (tokens int, full bool) {
	tokens, window, _, ok := e.deps.ContextUsage(project, worker)
	if !ok {
		return tokens, false
	}
	return tokens, Full(tokens, window)
}

// compactDue is fill past CompactionThreshold's curve, far below ContextFullFraction — a trigger,
// not the hard stop ContextFullFraction is.
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

// prepareAssignment runs an already-claimed assignment's preparation: a model switch the tier
// wants, else compaction if due, bracketed so AtLeafBoundary admits the claim just written. fired
// means the caller answers DirPreparing, not dir — dir is delivered by whatever fired instead.
func (e *Engine) prepareAssignment(project, agent, tier, dir string) (fired bool, err error) {
	e.deps.BeginAssignment(project, agent)
	defer e.deps.EndAssignment(project, agent)
	if want, known := e.deps.ModelForTier(tier); known && want != e.deps.CurrentModel(project, agent) {
		// The relaunch kills this reply regardless — dir is armed as its kickoff instead (-> kickoff.go).
		e.kickoff.arm(project, agent, dir)
		return true, e.deps.SetModel(project, agent, want)
	}
	return e.compactIfDue(project, agent, dir)
}

// ContextFull is contextFull's bool half, for the board's status word.
func (e *Engine) ContextFull(project, worker string) bool {
	_, full := e.contextFull(project, worker)
	return full
}
