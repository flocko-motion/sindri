// package: hub/workflow / fullness
// type:    logic (how full a worker's context is, and what that gates)
// job:     the one fact both the assignment gate and the board's status read (contextFull), the
// hard-stop fraction it retires a worker at (ContextFullFraction), and the much lower
// compaction bar the hub handles on its own (compactDue/containerCompactDue).
// limits:  the facts; withholding the next task on them is claimNext's, and the actual
// compaction is off-tick (-> FireDueCompactions).
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
// ContextFullFraction — AND an actual next task waiting to justify it (mirrors retierDue's own use
// of nextUp). Compacting is prep for a specific assignment about to be handed out, not a standing
// chore for whoever happens to be idle and full: an idle worker with nothing queued is left alone,
// the same way retierDue leaves its model alone with nothing to match. Withheld here; the actual
// firing is off-tick (-> FireDueCompactions).
func (e *Engine) compactDue(project, worker string) (tokens int, due bool) {
	tokens, window, _, ok := e.deps.ContextUsage(project, worker)
	if !ok || window <= 0 || tokens < e.deps.CompactionThreshold(window) {
		return tokens, false
	}
	// Synced here rather than assumed fresh: this may be the first thing on the request that reads
	// the cache, ahead of claimNext's own sync — a stale, empty cache would read as nothing queued.
	_ = e.SyncTasks(project)
	ps := e.store.For(project)
	packages, err := ps.OpenContainers()
	if err != nil {
		return tokens, false
	}
	leaves, err := ps.OpenLeaves()
	if err != nil {
		return tokens, false
	}
	_, _, ok = nextUp(packages, leaves, nil)
	return tokens, ok
}

// containerCompactDue is compactDue's question one level down: a held feature's OWN next subtask
// (mirrors containerRetierDue), for the worker already inside it rather than the top-level pools.
func (e *Engine) containerCompactDue(project, worker, container string) (tokens int, due bool) {
	tokens, window, _, ok := e.deps.ContextUsage(project, worker)
	if !ok || window <= 0 || tokens < e.deps.CompactionThreshold(window) {
		return tokens, false
	}
	_ = e.SyncTasks(project)
	children, err := e.store.For(project).OpenSubtasks(container)
	return tokens, err == nil && len(children) > 0
}

// ContextFull is contextFull's bool half, for the board's status word.
func (e *Engine) ContextFull(project, worker string) bool {
	_, full := e.contextFull(project, worker)
	return full
}
