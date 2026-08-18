// package: hub/workflow / fullness
// type:    logic (how full a worker's context is, and what that gates)
// job:     the one fact both the assignment gate and the board's status read (contextFull), the
// hard-stop fraction it retires a worker at (ContextFullFraction), and the much lower
// compaction bar the gate itself acts on inline (compactDue).
// limits:  the fill facts only; whether an assignment exists to compact ahead of is the
// caller's — claimNext and reviewDirective only ask once they already have one.
package workflow

// ContextFullFraction is how much of its window a worker may fill before it stops being handed new
// work. A fraction, not a token count: a flat 170k written for a 200k window retired workers on a
// 1M one with most of it unused.
const ContextFullFraction = 0.85

// Full is the fullness rule over figures the caller already holds. A window of 0 is never full — an
// unknown window must not retire anybody, since guessing one is what this replaced — which covers an
// agent with no recorded usage yet, whose window reads 0 too.
//
// Exported for the board, which has the figures from the watchdog's standing sample: asking the
// engine would make it take the transcript read again, per agent, per render (-> hub/state.go).
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

// compactDue is fill past CompactionThreshold's curve for the running model — far below
// ContextFullFraction. Whether that is worth acting on is the caller's to decide: claimNext and
// reviewDirective only ask once they already have a specific assignment to compact ahead of, so an
// idle agent with nothing queued is never told this is due — there is nothing left to check here.
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
