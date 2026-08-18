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
// not a gate: whether firing gets the agent back under it is not this function's business, or
// compactIfDue's either (-> ContextFullFraction, the actual hard stop).
func (e *Engine) compactDue(project, worker string) (tokens int, due bool) {
	tokens, window, _, ok := e.deps.ContextUsage(project, worker)
	return tokens, ok && window > 0 && tokens >= e.deps.CompactionThreshold(window)
}

// compactIfDue fires a due compaction — once, no retry, no re-reading the result to judge whether
// it "worked". A summary this far below the fullness gate may well still read due afterward; success
// isn't this call's to test, or a later claim's flag to skip past instead of firing again.
func (e *Engine) compactIfDue(project, agent string) error {
	if _, due := e.compactDue(project, agent); !due {
		return nil
	}
	return e.deps.Compact(project, agent)
}

// prepareAssignment runs an already-claimed assignment's preparation: a model switch the tier
// wants, else compaction if due (never both — SetModel clears the old session itself, -> agent.
// Service.SetModel). Bracketed by BeginAssignment/EndAssignment so AtLeafBoundary admits the claim
// just written, rather than refusing the preparation the gate is now running against its own guard.
func (e *Engine) prepareAssignment(project, agent, tier string) error {
	e.deps.BeginAssignment(project, agent)
	defer e.deps.EndAssignment(project, agent)
	if want, known := e.deps.ModelForTier(tier); known && want != e.deps.CurrentModel(project, agent) {
		return e.deps.SetModel(project, agent, want)
	}
	return e.compactIfDue(project, agent)
}

// ContextFull is contextFull's bool half, for the board's status word.
func (e *Engine) ContextFull(project, worker string) bool {
	_, full := e.contextFull(project, worker)
	return full
}
