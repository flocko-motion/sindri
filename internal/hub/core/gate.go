// package: hub/core / gate
// type:    assembly (the project's declared quality gate)
// job:     the gate command a project declares, and the validators the composition root installs.
// Held here because a submit, a contribute and a lint all ask the same question, and the answer is
// a project fact rather than any one subject's rule.
// limits:  holding and asking. What a failing gate MEANS is the asking subject's, and what a gate
// IS lives beside the handles it is installed on (-> core.go's Gate).
package core

// Gate is a quality check the submit path runs against a worktree before accepting its changes.
// Declared HERE, by what runs it, rather than beside an implementation: openspec implements it
// today and the built-in lint gate is next, so the submit path never names either concretely
// (-> ARCHITECTURE.md, on ports).
type Gate interface {
	// Validate checks wt; ok=false fails the submit, with output explaining why. A gate whose
	// domain doesn't apply to this repo (no openspec/ dir, nothing declared, ...) passes silently.
	Validate(wt string) (ok bool, output string)
}

// VerifyCmd is the project's declared gate command, "" when it declares none or the config cannot be
// read. Nothing stands in for it: an undeclared gate refuses every submit (-> adapter/git.Gate).
func (c *Core) VerifyCmd(project string) string {
	cfg, err := c.Deps.ProjectConfig(project)
	if err != nil {
		return ""
	}
	return cfg.Verify
}

// QualityGate runs every installed gate against wt, stopping at the first failure. A gate whose
// domain doesn't apply to this repo (e.g. openspec with no openspec/ dir) reports ok=true itself.
func (c *Core) QualityGate(wt string) (ok bool, output string) {
	for _, g := range c.gates {
		if ok, out := g.Validate(wt); !ok {
			return false, out
		}
	}
	return true, ""
}

// WithGates installs the submit path's quality gates, chainable alongside New. An engine with
// none runs no gate.
func (c *Core) WithGates(gates ...Gate) *Core {
	c.gates = gates
	return c
}
