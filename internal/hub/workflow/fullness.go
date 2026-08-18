// package: hub/workflow / fullness
// type:    logic (how full a worker's context is, and what that gates)
// job:     the hard-stop fullness fraction (ContextFullFraction) and the much lower compaction bar
// (compactDue), plus the pending flag its threshold crossing clears (compactOrWait).
// limits:  the fill facts; an assignment to compact ahead of is the caller's, asked once it has one.
package workflow

// ContextFullFraction is how much of its window a worker may fill before it stops being handed new
// work. A fraction, not a token count: a flat 170k written for a 200k window retired workers on a
// 1M one with most of it unused.
const ContextFullFraction = 0.85

// Full is the fullness rule over figures the caller already holds, exported for the board (which
// has them from the watchdog's own sample, sparing a transcript read per render). A window of 0 —
// no recorded usage yet — is never full; guessing one is what this replaced.
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

// compactDue is fill past CompactionThreshold's curve — far below ContextFullFraction. Not due,
// for ANY reason including an unreadable or unknown-window sample, is also the only signal that
// clears a pending compaction's flag — miss one such reading (the early return used to) and a
// transient blip strands it true, with no timer to retire it, forever after.
func (e *Engine) compactDue(project, worker string) (tokens int, due bool) {
	tokens, window, _, ok := e.deps.ContextUsage(project, worker)
	due = ok && window > 0 && tokens >= e.deps.CompactionThreshold(window)
	if !due {
		e.deps.ForgetCompactPending(project, worker)
	}
	return tokens, due
}

// compactOrWait is claimNext's shared step, used by its two siblings too: fire a due compaction, or
// report one already queued — either way the pass ends here, so an assignment never lands in the
// context it exists to avoid. A re-ask can't loop this forever: Claude Code drains its queue one
// turn at a time, so /compact's own turn — and compactDue's threshold with it — always lands before
// the kickoff behind it can ask again.
func (e *Engine) compactOrWait(project, agent string) (dir string, acted bool, err error) {
	if _, due := e.compactDue(project, agent); !due {
		return "", false, nil
	}
	if !e.deps.CompactPending(project, agent) {
		if err := e.deps.Compact(project, agent); err != nil {
			return "", false, err
		}
	}
	return DirCompacting, true, nil
}

// ContextFull is contextFull's bool half, for the board's status word.
func (e *Engine) ContextFull(project, worker string) bool {
	_, full := e.contextFull(project, worker)
	return full
}
