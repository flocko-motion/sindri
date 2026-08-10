// package: adapter/gate / gate
// type:    logic (the quality-gate PORT)
// job:     the generic interface a submit-path quality check implements — validate a
// worktree and report pass/fail plus why. Openspec implements it today; the
// built-in lint gate (-> hub/repo.Gate) is next, so the submit path never
// names either concretely.
// limits:  no hub policy here — a Gate just checks a worktree and reports the verdict.
package gate

// Gate is a quality check the submit path runs against a worktree before accepting its changes.
type Gate interface {
	// Name identifies the gate for logging/output.
	Name() string
	// Validate checks wt; ok=false fails the submit, with output explaining why. A gate whose
	// domain doesn't apply to this repo (no openspec/ dir, nothing declared, ...) passes silently.
	Validate(wt string) (ok bool, output string)
}
