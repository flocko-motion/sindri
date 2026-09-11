// package: hub/flow/agent / fullness
// type:    logic (how full a worker's context is, and what that gates)
// job:     the hard-stop fullness fraction, and how full a worker's session actually is.
// limits:  the facts. PREPARING a session — a model switch, else a clear — is a state the machine
// stands the agent in (-> roles/lifecycle's Clearing, worker's Retiering), never a step inside
// whatever is about to hand it work.
package agent

// ContextFullFraction is how much of its window a worker may fill before a fresh assignment
// clears it — a session that far gone carries nothing worth keeping.
const ContextFullFraction = 0.85

// ContextFull reports whether a worker is past ContextFullFraction of its window. A window of 0,
// or no recorded usage yet, is never full — guessing one is what this replaced.
func (a *Act) ContextFull(project, worker string) (tokens int, full bool) {
	o := a.Harness.Observe(project, worker)
	return o.Fill, o.Window > 0 && float64(o.Fill) >= float64(o.Window)*ContextFullFraction
}
