// package: hub/workflow / fullness
// type:    logic (how full a worker's context is, and what that gates)
// job:     the one fact both the assignment gate and the board's status read (contextFull), the
// hard-stop fraction it retires a worker at (ContextFullFraction), and the much lower
// compaction bar the hub handles on its own (compactDue).
// limits:  the facts; withholding the next task on them is claimNext's, and the actual
// compaction is off-tick (-> agent.FireDueCompactions).
package workflow

// ContextFullFraction is how much of its window a worker may fill before it stops being handed new
// work. A fraction, not a token count: a flat 170k written for a 200k window retired workers on a
// 1M one with most of it unused.
const ContextFullFraction = 0.85

// contextFull is the one fact both the assignment gate and the board's status read. No recorded
// usage yet (ok=false from ContextUsage) is never full, and neither is a window of 0 — an unknown
// window must not retire anybody, since guessing one is what this replaced.
func (e *Engine) contextFull(project, worker string) (tokens int, full bool) {
	tokens, window, _, ok := e.deps.ContextUsage(project, worker)
	if !ok || window <= 0 {
		return tokens, false
	}
	return tokens, float64(tokens) >= float64(window)*ContextFullFraction
}

// compactDue is fill past CompactionThreshold's curve for the running model — far below
// ContextFullFraction. Withheld here; the actual firing is off-tick (-> agent.FireDueCompactions).
func (e *Engine) compactDue(project, worker string) (tokens int, due bool) {
	tokens, window, _, ok := e.deps.ContextUsage(project, worker)
	if !ok || window <= 0 {
		return tokens, false
	}
	return tokens, tokens >= e.deps.CompactionThreshold(window)
}

// ContextFull is contextFull's bool half, for the board's status word.
func (e *Engine) ContextFull(project, worker string) bool {
	_, full := e.contextFull(project, worker)
	return full
}
